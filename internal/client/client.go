// Package client is a thin HTTP client for the Vanta REST API (v1).
//
// It differs from a plain API-key client in two ways that shape the rest of
// the package:
//
//  1. Authentication is OAuth 2.0 client-credentials. The caller supplies a
//     client_id/client_secret pair; the client exchanges them for a short-lived
//     bearer token at the token URL and refreshes it transparently before it
//     expires. A pre-minted static token may be supplied instead (handy for
//     tests and for callers that manage their own token lifecycle).
//  2. List endpoints are cursor-paginated and wrap their payload in a
//     {"results": {"data": [...], "pageInfo": {...}}} envelope. The generic
//     paginate helper walks every page so resource code sees a flat slice.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// RegionUS is Vanta's commercial deployment.
	RegionUS = "us"
	// RegionGov is Vanta Gov (FedRAMP).
	RegionGov = "gov"
)

type regionEndpoints struct {
	baseURL  string
	tokenURL string
}

var regionURLs = map[string]regionEndpoints{
	RegionUS:  {baseURL: "https://api.vanta.com/v1", tokenURL: "https://api.vanta.com/oauth/token"},
	RegionGov: {baseURL: "https://api.vanta-gov.com/v1", tokenURL: "https://api.vanta-gov.com/oauth/token"},
}

// DefaultScope grants read+write across all data. Narrow it via Options.Scope
// to follow least-privilege.
const DefaultScope = "vanta-api.all:read vanta-api.all:write"

type Client struct {
	httpClient *http.Client
	baseURL    *url.URL
	userAgent  string

	// OAuth client-credentials inputs.
	tokenURL     string
	clientID     string
	clientSecret string
	scope        string

	maxRetries   int
	maxRetryWait time.Duration

	// Client-side request pacing. When minInterval > 0, Do spaces the start of
	// every request (list pages, per-id reads, writes) by at least minInterval,
	// shared across all goroutines. This keeps a bulk apply/import — which
	// Terraform fans out at its configured parallelism — under Vanta's rate
	// limit instead of relying on 429 retries after the fact. Zero disables it.
	minInterval time.Duration
	rateMu      sync.Mutex
	nextAllowed time.Time

	// Token cache. A static token short-circuits the exchange.
	staticToken string
	tokenMu     sync.Mutex
	accessToken string
	tokenExpiry time.Time

	// Vendor list cache for name lookups. Populated lazily on the first
	// GetVendorByName call and reused, so a bulk apply/import that resolves many
	// vendors by name issues a single (paginated) list request instead of one
	// per vendor. See GetVendorByName.
	vendorMu     sync.Mutex
	vendorList   []Vendor
	vendorCached bool
}

type Options struct {
	// ClientID / ClientSecret drive the OAuth client-credentials exchange.
	// Ignored when Token is set.
	ClientID     string
	ClientSecret string
	Scope        string

	// Token is a pre-obtained bearer token. When set, no OAuth exchange is
	// performed and ClientID/ClientSecret are ignored.
	Token string

	// Region selects Vanta's commercial ("us") or FedRAMP ("gov") hosts.
	// Ignored when BaseURL/TokenURL are set explicitly.
	Region string

	// BaseURL / TokenURL override the region-derived endpoints (used by tests
	// and self-hosted proxies).
	BaseURL  string
	TokenURL string

	UserAgent string

	Timeout      time.Duration
	MaxRetries   int
	MaxRetryWait time.Duration

	// RequestsPerSecond caps the client's request rate across all concurrent
	// callers. A value <= 0 disables pacing (the default, so tests run at full
	// speed); the provider sets a sane positive default for real usage.
	RequestsPerSecond float64
}

func New(opts Options) (*Client, error) {
	if opts.Token == "" && (opts.ClientID == "" || opts.ClientSecret == "") {
		return nil, errors.New("either token or both client_id and client_secret are required")
	}

	region := strings.ToLower(opts.Region)
	if region == "" {
		region = RegionUS
	}
	endpoints, ok := regionURLs[region]
	if !ok {
		return nil, fmt.Errorf("unknown region %q (expected us or gov)", region)
	}

	baseRaw := opts.BaseURL
	if baseRaw == "" {
		baseRaw = endpoints.baseURL
	}
	base, err := url.Parse(baseRaw)
	if err != nil {
		return nil, fmt.Errorf("invalid base url %q: %w", baseRaw, err)
	}

	tokenURL := opts.TokenURL
	if tokenURL == "" {
		tokenURL = endpoints.tokenURL
	}

	scope := opts.Scope
	if scope == "" {
		scope = DefaultScope
	}
	timeout := opts.Timeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}
	maxRetries := opts.MaxRetries
	if maxRetries == 0 {
		maxRetries = 5
	}
	maxRetryWait := opts.MaxRetryWait
	if maxRetryWait == 0 {
		maxRetryWait = 60 * time.Second
	}
	var minInterval time.Duration
	if opts.RequestsPerSecond > 0 {
		minInterval = time.Duration(float64(time.Second) / opts.RequestsPerSecond)
	}

	return &Client{
		httpClient:   &http.Client{Timeout: timeout},
		baseURL:      base,
		userAgent:    opts.UserAgent,
		tokenURL:     tokenURL,
		clientID:     opts.ClientID,
		clientSecret: opts.ClientSecret,
		scope:        scope,
		maxRetries:   maxRetries,
		maxRetryWait: maxRetryWait,
		minInterval:  minInterval,
		staticToken:  opts.Token,
	}, nil
}

// APIError represents a non-2xx response.
type APIError struct {
	StatusCode int
	Message    string
	Body       string
	RetryAfter string // For 429 responses.
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("vanta api error (status %d): %s", e.StatusCode, e.Message)
	}
	return fmt.Sprintf("vanta api error (status %d): %s", e.StatusCode, e.Body)
}

func (e *APIError) IsNotFound() bool { return e.StatusCode == http.StatusNotFound }

// IsNotFound reports whether err is (or wraps) a 404 APIError.
func IsNotFound(err error) bool {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.IsNotFound()
	}
	return false
}

// token returns a valid bearer token, performing (or refreshing) the OAuth
// client-credentials exchange as needed. It is safe for concurrent use.
func (c *Client) token(ctx context.Context) (string, error) {
	if c.staticToken != "" {
		return c.staticToken, nil
	}

	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()

	// Reuse a cached token while it has at least 30s of life left.
	if c.accessToken != "" && time.Now().Add(30*time.Second).Before(c.tokenExpiry) {
		return c.accessToken, nil
	}

	reqBody, err := json.Marshal(map[string]any{
		"grant_type":    "client_credentials",
		"client_id":     c.clientID,
		"client_secret": c.clientSecret,
		"scope":         c.scope,
	})
	if err != nil {
		return "", fmt.Errorf("marshal token request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.tokenURL, bytes.NewReader(reqBody))
	if err != nil {
		return "", fmt.Errorf("build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if c.userAgent != "" {
		req.Header.Set("User-Agent", c.userAgent)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("request oauth token: %w", err)
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return "", fmt.Errorf("read token response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return "", &APIError{StatusCode: resp.StatusCode, Body: string(body), Message: "oauth token request failed"}
	}

	var token struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &token); err != nil {
		return "", fmt.Errorf("decode token response: %w", err)
	}
	if token.AccessToken == "" {
		return "", errors.New("oauth token response did not contain an access_token")
	}

	c.accessToken = token.AccessToken
	expiresIn := token.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = 3600
	}
	c.tokenExpiry = time.Now().Add(time.Duration(expiresIn) * time.Second)
	return c.accessToken, nil
}

// Do is the core request method: it handles auth, JSON (un)marshaling, 429
// retries, and error parsing. body and out may be nil.
func (c *Client) Do(ctx context.Context, method, path string, query url.Values, body, out any) error {
	u := *c.baseURL
	u.Path = strings.TrimRight(u.Path, "/") + "/" + strings.TrimLeft(path, "/")
	if len(query) > 0 {
		u.RawQuery = query.Encode()
	}

	var bodyBytes []byte
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request body: %w", err)
		}
		bodyBytes = b
	}

	for attempt := 0; ; attempt++ {
		if err := c.waitRate(ctx); err != nil {
			return err
		}

		tok, err := c.token(ctx)
		if err != nil {
			return err
		}

		var reqBody io.Reader
		if bodyBytes != nil {
			reqBody = bytes.NewReader(bodyBytes)
		}
		req, err := http.NewRequestWithContext(ctx, method, u.String(), reqBody)
		if err != nil {
			return fmt.Errorf("build request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+tok)
		req.Header.Set("Accept", "application/json")
		if reqBody != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if c.userAgent != "" {
			req.Header.Set("User-Agent", c.userAgent)
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("send request: %w", err)
		}
		respBody, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return fmt.Errorf("read response body: %w", err)
		}

		if resp.StatusCode == http.StatusTooManyRequests && attempt < c.maxRetries {
			wait := retryWait(resp.Header.Get("Retry-After"), attempt, c.maxRetryWait)
			select {
			case <-time.After(wait):
				continue
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		if resp.StatusCode >= 400 {
			apiErr := &APIError{StatusCode: resp.StatusCode, Body: string(respBody)}
			var parsed struct {
				Message string `json:"message"`
				Error   string `json:"error"`
			}
			if json.Unmarshal(respBody, &parsed) == nil {
				if parsed.Message != "" {
					apiErr.Message = parsed.Message
				} else if parsed.Error != "" {
					apiErr.Message = parsed.Error
				}
			}
			if resp.StatusCode == http.StatusTooManyRequests {
				apiErr.RetryAfter = resp.Header.Get("Retry-After")
			}
			return apiErr
		}

		if out == nil || len(respBody) == 0 {
			return nil
		}
		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
		return nil
	}
}

// waitRate blocks until the client's shared rate gate allows another request,
// spacing successive requests by at least minInterval regardless of how many
// goroutines call it. It reserves the next slot under rateMu and then sleeps
// (outside the lock) so callers queue in order without serializing the waits.
// A cancelled context aborts the wait. It is a no-op when pacing is disabled.
func (c *Client) waitRate(ctx context.Context) error {
	if c.minInterval <= 0 {
		return nil
	}

	c.rateMu.Lock()
	now := time.Now()
	start := c.nextAllowed
	if start.Before(now) {
		start = now
	}
	c.nextAllowed = start.Add(c.minInterval)
	c.rateMu.Unlock()

	wait := time.Until(start)
	if wait <= 0 {
		return nil
	}
	select {
	case <-time.After(wait):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// retryWait picks how long to wait before retrying a 429. It honors a
// server-supplied Retry-After header when present; otherwise it backs off
// exponentially (0.5s, 1s, 2s, ...) so a burst that overruns the rate gate
// still converges instead of hammering with a fixed 1s delay. The result is
// clamped to maxWait.
func retryWait(retryAfter string, attempt int, maxWait time.Duration) time.Duration {
	if retryAfter != "" {
		return parseRetryAfter(retryAfter, maxWait)
	}
	wait := 500 * time.Millisecond << attempt
	if wait > maxWait {
		return maxWait
	}
	return wait
}

// pageInfo mirrors the cursor metadata Vanta returns on every list endpoint.
type pageInfo struct {
	EndCursor   *string `json:"endCursor"`
	HasNextPage bool    `json:"hasNextPage"`
}

// paginatedResponse is the {"results": {"data": [...], "pageInfo": {...}}}
// envelope every list endpoint returns.
type paginatedResponse[T any] struct {
	Results struct {
		Data     []T      `json:"data"`
		PageInfo pageInfo `json:"pageInfo"`
	} `json:"results"`
}

// paginate walks every page of a cursor-paginated list endpoint and returns the
// concatenated items. extraQuery carries endpoint-specific filters; the helper
// owns pageSize/pageCursor.
func paginate[T any](ctx context.Context, c *Client, path string, extraQuery url.Values) ([]T, error) {
	var all []T
	var cursor string
	for {
		q := url.Values{}
		for k, vs := range extraQuery {
			for _, v := range vs {
				q.Add(k, v)
			}
		}
		q.Set("pageSize", "100")
		if cursor != "" {
			q.Set("pageCursor", cursor)
		}

		var page paginatedResponse[T]
		if err := c.Do(ctx, http.MethodGet, path, q, nil, &page); err != nil {
			return nil, err
		}
		all = append(all, page.Results.Data...)
		if !page.Results.PageInfo.HasNextPage || page.Results.PageInfo.EndCursor == nil || *page.Results.PageInfo.EndCursor == "" {
			break
		}
		cursor = *page.Results.PageInfo.EndCursor
	}
	return all, nil
}

// parseRetryAfter handles both delta-seconds and HTTP-date forms.
func parseRetryAfter(value string, maxWait time.Duration) time.Duration {
	clamp := func(d time.Duration) time.Duration {
		if d < 0 {
			return 0
		}
		if d > maxWait {
			return maxWait
		}
		return d
	}
	if value == "" {
		return time.Second
	}
	if n, err := strconv.Atoi(strings.TrimSpace(value)); err == nil {
		return clamp(time.Duration(n) * time.Second)
	}
	if t, err := http.ParseTime(value); err == nil {
		return clamp(time.Until(t))
	}
	return time.Second
}
