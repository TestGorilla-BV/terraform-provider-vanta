// Package mockserver provides an in-memory HTTP server that simulates the
// subset of the Vanta API exercised by the provider's acceptance tests,
// including the OAuth client-credentials token endpoint. It does NOT aim for
// full API fidelity — only the request/response shape the provider depends on.
package mockserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
)

// Server wraps an httptest.Server and maintains in-memory state per resource.
type Server struct {
	*httptest.Server

	mu         sync.Mutex
	nextID     int
	vendors    map[string]map[string]any
	findings   map[string]map[string]map[string]any // vendorID -> findingID -> finding
	people     []map[string]any
	frameworks []map[string]any
	tests      []map[string]any
}

// New starts a mock server and returns it. Call srv.Close when done.
func New() *Server {
	s := &Server{
		nextID:   1,
		vendors:  map[string]map[string]any{},
		findings: map[string]map[string]map[string]any{},
		people: []map[string]any{
			{
				"id":           "person-1",
				"emailAddress": "alice@example.com",
				"name":         map[string]any{"first": "Alice", "last": "Example", "display": "Alice Example"},
				"employment":   map[string]any{"status": "CURRENT", "jobTitle": "Engineer"},
			},
			{
				"id":           "person-2",
				"emailAddress": "bob@example.com",
				"name":         map[string]any{"first": "Bob", "last": "Example", "display": "Bob Example"},
				"employment":   map[string]any{"status": "FORMER", "jobTitle": nil},
			},
		},
		frameworks: []map[string]any{
			{
				"id": "soc2", "displayName": "SOC 2", "shorthandName": "SOC 2", "description": "SOC 2 framework",
				"numControlsCompleted": 43, "numControlsTotal": 86, "numTestsPassing": 21, "numTestsTotal": 46,
			},
		},
		tests: []map[string]any{
			{"id": "test-1", "name": "Example test", "status": "OK", "category": "Account security", "lastTestRunDate": "2024-06-18T20:17:38.463Z"},
		},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /oauth/token", s.handleToken)
	mux.HandleFunc("GET /v1/vendors", s.handleListVendors)
	mux.HandleFunc("POST /v1/vendors", s.handleCreateVendor)
	mux.HandleFunc("GET /v1/vendors/{id}", s.handleGetVendor)
	mux.HandleFunc("PATCH /v1/vendors/{id}", s.handleUpdateVendor)
	mux.HandleFunc("DELETE /v1/vendors/{id}", s.handleDeleteVendor)
	mux.HandleFunc("GET /v1/vendors/{vendorId}/findings", s.handleListFindings)
	mux.HandleFunc("POST /v1/vendors/{vendorId}/findings", s.handleCreateFinding)
	mux.HandleFunc("PATCH /v1/vendors/{vendorId}/findings/{findingId}", s.handleUpdateFinding)
	mux.HandleFunc("DELETE /v1/vendors/{vendorId}/findings/{findingId}", s.handleDeleteFinding)
	mux.HandleFunc("GET /v1/people", s.handleListPeople)
	mux.HandleFunc("GET /v1/frameworks", s.handleListFrameworks)
	mux.HandleFunc("GET /v1/tests", s.handleListTests)

	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The token endpoint is unauthenticated; everything else requires a
		// bearer token.
		if !strings.HasPrefix(r.URL.Path, "/oauth/") {
			if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
				respondError(w, http.StatusUnauthorized, "missing bearer token")
				return
			}
		}
		mux.ServeHTTP(w, r)
	}))
	return s
}

// BaseURL returns the value to pass as VANTA_BASE_URL.
func (s *Server) BaseURL() string { return s.URL + "/v1" }

// TokenURL returns the value to pass as VANTA_TOKEN_URL.
func (s *Server) TokenURL() string { return s.URL + "/oauth/token" }

func (s *Server) nextResourceID(prefix string) string {
	id := s.nextID
	s.nextID++
	return prefix + "-" + strconv.Itoa(id)
}

func respondJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func respondError(w http.ResponseWriter, status int, msg string) {
	respondJSON(w, status, map[string]any{"message": msg})
}

func listEnvelope(data []any) map[string]any {
	return map[string]any{
		"results": map[string]any{
			"data":     data,
			"pageInfo": map[string]any{"endCursor": nil, "hasNextPage": false},
		},
	}
}

// ----- OAuth -----

func (s *Server) handleToken(w http.ResponseWriter, _ *http.Request) {
	respondJSON(w, http.StatusOK, map[string]any{
		"access_token": "mock-access-token",
		"token_type":   "Bearer",
		"expires_in":   3600,
	})
}

// VendorCount returns the number of vendors currently stored. Test helper.
func (s *Server) VendorCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.vendors)
}

// VendorCountByStatus returns the number of stored vendors with the given
// status. Test helper.
func (s *Server) VendorCountByStatus(status string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, v := range s.vendors {
		st, _ := vendorResponse(v)["status"].(string)
		if st == status {
			n++
		}
	}
	return n
}

// ----- Vendors -----

func (s *Server) handleCreateVendor(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	id := s.nextResourceID("vendor")
	body["id"] = id
	s.vendors[id] = body
	respondJSON(w, http.StatusOK, vendorResponse(body))
}

func (s *Server) handleListVendors(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	nameFilter := strings.ToLower(r.URL.Query().Get("name"))
	statuses := map[string]bool{}
	for _, st := range r.URL.Query()["statusMatchesAny"] {
		statuses[st] = true
	}
	data := make([]any, 0, len(s.vendors))
	for _, v := range s.vendors {
		shaped := vendorResponse(v)
		if nameFilter != "" {
			name, _ := shaped["name"].(string)
			if !strings.Contains(strings.ToLower(name), nameFilter) {
				continue
			}
		}
		if len(statuses) > 0 {
			st, _ := shaped["status"].(string)
			if !statuses[st] {
				continue
			}
		}
		data = append(data, shaped)
	}
	respondJSON(w, http.StatusOK, listEnvelope(data))
}

func (s *Server) handleGetVendor(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.vendors[r.PathValue("id")]
	if !ok {
		respondError(w, http.StatusNotFound, "vendor not found")
		return
	}
	respondJSON(w, http.StatusOK, vendorResponse(v))
}

func (s *Server) handleUpdateVendor(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.vendors[r.PathValue("id")]
	if !ok {
		respondError(w, http.StatusNotFound, "vendor not found")
		return
	}
	for k, val := range body {
		v[k] = val
	}
	respondJSON(w, http.StatusOK, vendorResponse(v))
}

func (s *Server) handleDeleteVendor(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := r.PathValue("id")
	if _, ok := s.vendors[id]; !ok {
		respondError(w, http.StatusNotFound, "vendor not found")
		return
	}
	delete(s.vendors, id)
	delete(s.findings, id)
	w.WriteHeader(http.StatusNoContent)
}

// vendorResponse fills in server-defaulted fields and reshapes the category
// string into the object the read API returns.
func vendorResponse(v map[string]any) map[string]any {
	out := map[string]any{}
	for k, val := range v {
		out[k] = val
	}
	if _, ok := out["status"]; !ok {
		out["status"] = "MANAGED"
	}
	if _, ok := out["inherentRiskLevel"]; !ok {
		out["inherentRiskLevel"] = "UNSCORED"
	}
	if _, ok := out["residualRiskLevel"]; !ok {
		out["residualRiskLevel"] = "UNSCORED"
	}
	if c, ok := out["category"].(string); ok {
		out["category"] = map[string]any{"displayName": c}
	}
	return out
}

// ----- Vendor findings -----

func (s *Server) handleCreateFinding(w http.ResponseWriter, r *http.Request) {
	vendorID := r.PathValue("vendorId")
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.vendors[vendorID]; !ok {
		respondError(w, http.StatusNotFound, "vendor not found")
		return
	}
	id := s.nextResourceID("finding")
	body["id"] = id
	body["vendorId"] = vendorID
	if s.findings[vendorID] == nil {
		s.findings[vendorID] = map[string]map[string]any{}
	}
	s.findings[vendorID][id] = body
	respondJSON(w, http.StatusOK, body)
}

func (s *Server) handleListFindings(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data := make([]any, 0)
	for _, f := range s.findings[r.PathValue("vendorId")] {
		data = append(data, f)
	}
	respondJSON(w, http.StatusOK, listEnvelope(data))
}

func (s *Server) handleUpdateFinding(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	vendorID, findingID := r.PathValue("vendorId"), r.PathValue("findingId")
	f, ok := s.findings[vendorID][findingID]
	if !ok {
		respondError(w, http.StatusNotFound, "finding not found")
		return
	}
	for k, val := range body {
		f[k] = val
	}
	respondJSON(w, http.StatusOK, f)
}

func (s *Server) handleDeleteFinding(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	vendorID, findingID := r.PathValue("vendorId"), r.PathValue("findingId")
	if _, ok := s.findings[vendorID][findingID]; !ok {
		respondError(w, http.StatusNotFound, "finding not found")
		return
	}
	delete(s.findings[vendorID], findingID)
	w.WriteHeader(http.StatusNoContent)
}

// ----- Read-only collections -----

func (s *Server) handleListPeople(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("employmentStatus")
	data := make([]any, 0, len(s.people))
	for _, p := range s.people {
		if status != "" {
			emp, _ := p["employment"].(map[string]any)
			if emp["status"] != status {
				continue
			}
		}
		data = append(data, p)
	}
	respondJSON(w, http.StatusOK, listEnvelope(data))
}

func (s *Server) handleListFrameworks(w http.ResponseWriter, _ *http.Request) {
	data := make([]any, 0, len(s.frameworks))
	for _, f := range s.frameworks {
		data = append(data, f)
	}
	respondJSON(w, http.StatusOK, listEnvelope(data))
}

func (s *Server) handleListTests(w http.ResponseWriter, _ *http.Request) {
	data := make([]any, 0, len(s.tests))
	for _, t := range s.tests {
		data = append(data, t)
	}
	respondJSON(w, http.StatusOK, listEnvelope(data))
}
