package client

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
)

// Vendor is the subset of Vanta's vendor object that the provider manages. The
// full API object has many more fields (auth details, contract amounts, custom
// fields, decisions); they are intentionally omitted here and can be added
// incrementally without changing the request/response plumbing.
type Vendor struct {
	ID                      string  `json:"id"`
	Name                    string  `json:"name"`
	WebsiteURL              *string `json:"websiteUrl"`
	AccountManagerName      *string `json:"accountManagerName"`
	AccountManagerEmail     *string `json:"accountManagerEmail"`
	ServicesProvided        *string `json:"servicesProvided"`
	AdditionalNotes         *string `json:"additionalNotes"`
	SecurityOwnerUserID     *string `json:"securityOwnerUserId"`
	BusinessOwnerUserID     *string `json:"businessOwnerUserId"`
	ContractStartDate       *string `json:"contractStartDate"`
	ContractRenewalDate     *string `json:"contractRenewalDate"`
	ContractTerminationDate *string `json:"contractTerminationDate"`
	IsVisibleToAuditors     *bool   `json:"isVisibleToAuditors"`
	Status                  string  `json:"status"`
	InherentRiskLevel       string  `json:"inherentRiskLevel"`
	ResidualRiskLevel       string  `json:"residualRiskLevel"`
	VendorHeadquarters      *string `json:"vendorHeadquarters"`
	Category                *struct {
		DisplayName string `json:"displayName"`
	} `json:"category"`
	ContractAmount *VendorContractAmount `json:"contractAmount"`
	AuthDetails    *struct {
		Method *string `json:"method"`
	} `json:"authDetails"`
	// Computed, read-only.
	NextSecurityReviewDueDate        *string `json:"nextSecurityReviewDueDate"`
	LastSecurityReviewCompletionDate *string `json:"lastSecurityReviewCompletionDate"`
}

// VendorContractAmount is the vendor's contract value. Amount is the numeric
// value; Currency is an ISO 4217 code the API accepts (e.g. USD, EUR).
type VendorContractAmount struct {
	Amount   float64 `json:"amount"`
	Currency string  `json:"currency"`
}

// AuthenticationMethod returns the vendor's authentication method, or "" if
// unset.
func (v *Vendor) AuthenticationMethod() string {
	if v.AuthDetails == nil || v.AuthDetails.Method == nil {
		return ""
	}
	return *v.AuthDetails.Method
}

// CategoryDisplayName returns the vendor's category name, or "" if unset.
func (v *Vendor) CategoryDisplayName() string {
	if v.Category == nil {
		return ""
	}
	return v.Category.DisplayName
}

// VendorInput is the create/update payload. Pointer fields are only serialized
// when non-nil, so unset attributes are left untouched on PATCH.
type VendorInput struct {
	Name                    *string                 `json:"name,omitempty"`
	WebsiteURL              *string                 `json:"websiteUrl,omitempty"`
	AccountManagerName      *string                 `json:"accountManagerName,omitempty"`
	AccountManagerEmail     *string                 `json:"accountManagerEmail,omitempty"`
	ServicesProvided        *string                 `json:"servicesProvided,omitempty"`
	AdditionalNotes         *string                 `json:"additionalNotes,omitempty"`
	SecurityOwnerUserID     *string                 `json:"securityOwnerUserId,omitempty"`
	BusinessOwnerUserID     *string                 `json:"businessOwnerUserId,omitempty"`
	ContractStartDate       *string                 `json:"contractStartDate,omitempty"`
	ContractRenewalDate     *string                 `json:"contractRenewalDate,omitempty"`
	ContractTerminationDate *string                 `json:"contractTerminationDate,omitempty"`
	IsVisibleToAuditors     *bool                   `json:"isVisibleToAuditors,omitempty"`
	Status                  *string                 `json:"status,omitempty"`
	Category                *string                 `json:"category,omitempty"`
	InherentRiskLevel       *string                 `json:"inherentRiskLevel,omitempty"`
	ResidualRiskLevel       *string                 `json:"residualRiskLevel,omitempty"`
	VendorHeadquarters      *string                 `json:"vendorHeadquarters,omitempty"`
	ContractAmount          *VendorContractAmount   `json:"contractAmount,omitempty"`
	AuthDetails             *VendorAuthDetailsInput `json:"authDetails,omitempty"`
}

// VendorAuthDetailsInput is the writable subset of a vendor's authentication
// details. Only the method is managed here; the API leaves the other auth
// fields untouched when they are omitted.
type VendorAuthDetailsInput struct {
	Method *string `json:"method,omitempty"`
}

func (c *Client) CreateVendor(ctx context.Context, input VendorInput) (*Vendor, error) {
	var out Vendor
	if err := c.Do(ctx, http.MethodPost, "/vendors", nil, input, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) GetVendor(ctx context.Context, id string) (*Vendor, error) {
	var out Vendor
	if err := c.Do(ctx, http.MethodGet, "/vendors/"+url.PathEscape(id), nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) UpdateVendor(ctx context.Context, id string, input VendorInput) (*Vendor, error) {
	var out Vendor
	if err := c.Do(ctx, http.MethodPatch, "/vendors/"+url.PathEscape(id), nil, input, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) DeleteVendor(ctx context.Context, id string) error {
	return c.Do(ctx, http.MethodDelete, "/vendors/"+url.PathEscape(id), nil, nil, nil)
}

// ListVendorsFilter narrows the vendor list. Zero values are omitted.
type ListVendorsFilter struct {
	Name             string
	StatusMatchesAny []string
}

func (c *Client) ListVendors(ctx context.Context, filter ListVendorsFilter) ([]Vendor, error) {
	q := url.Values{}
	if filter.Name != "" {
		q.Set("name", filter.Name)
	}
	for _, s := range filter.StatusMatchesAny {
		q.Add("statusMatchesAny", s)
	}
	return paginate[Vendor](ctx, c, "/vendors", q)
}

// cachedVendors returns the full vendor list, fetching and caching it on the
// first call. The cache is shared across concurrent callers so a bulk
// apply/import resolves every vendor from a single (paginated) list request
// instead of one request per vendor.
func (c *Client) cachedVendors(ctx context.Context) ([]Vendor, error) {
	c.vendorMu.Lock()
	defer c.vendorMu.Unlock()
	if c.vendorCached {
		return c.vendorList, nil
	}
	vendors, err := c.ListVendors(ctx, ListVendorsFilter{})
	if err != nil {
		return nil, err
	}
	c.vendorList = vendors
	c.vendorCached = true
	return c.vendorList, nil
}

// GetVendorByName looks up a single managed vendor by exact name. Returns a
// NotFound APIError when absent and a generic error when the name is ambiguous.
//
// It matches against the cached full vendor list (see cachedVendors) rather
// than issuing a per-name list request, so many name lookups during one
// apply/import don't hammer the API.
func (c *Client) GetVendorByName(ctx context.Context, name string) (*Vendor, error) {
	vendors, err := c.cachedVendors(ctx)
	if err != nil {
		return nil, err
	}
	var matches []Vendor
	for i := range vendors {
		if vendors[i].Name == name {
			matches = append(matches, vendors[i])
		}
	}
	switch len(matches) {
	case 0:
		return nil, &APIError{StatusCode: http.StatusNotFound, Message: fmt.Sprintf("vendor with name %q not found", name)}
	case 1:
		return &matches[0], nil
	default:
		return nil, fmt.Errorf("vendor name %q is ambiguous: %d vendors share it; import by ID instead", name, len(matches))
	}
}
