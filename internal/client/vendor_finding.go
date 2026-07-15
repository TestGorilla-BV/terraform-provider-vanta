package client

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
)

// VendorFinding is a finding attached to a vendor (typically surfaced during a
// security review). Remediation is only populated when RiskStatus is REMEDIATE.
type VendorFinding struct {
	ID               string       `json:"id"`
	VendorID         string       `json:"vendorId"`
	SecurityReviewID *string      `json:"securityReviewId"`
	DocumentID       *string      `json:"documentId"`
	Content          string       `json:"content"`
	RiskStatus       string       `json:"riskStatus"`
	Remediation      *Remediation `json:"remediation"`
}

type Remediation struct {
	State            string  `json:"state"`
	RequirementNotes *string `json:"requirementNotes"`
}

// CreateFindingInput is the POST body. SecurityReviewID/DocumentID are optional
// links; Remediation is only meaningful when RiskStatus == "REMEDIATE".
type CreateFindingInput struct {
	Content          string       `json:"content"`
	RiskStatus       string       `json:"riskStatus"`
	Remediation      *Remediation `json:"remediation,omitempty"`
	SecurityReviewID *string      `json:"securityReviewId,omitempty"`
	DocumentID       *string      `json:"documentId,omitempty"`
}

// UpdateFindingInput is the PATCH body.
type UpdateFindingInput struct {
	Content     *string      `json:"content,omitempty"`
	RiskStatus  *string      `json:"riskStatus,omitempty"`
	Remediation *Remediation `json:"remediation,omitempty"`
}

func (c *Client) CreateVendorFinding(ctx context.Context, vendorID string, input CreateFindingInput) (*VendorFinding, error) {
	var out VendorFinding
	path := fmt.Sprintf("/vendors/%s/findings", url.PathEscape(vendorID))
	if err := c.Do(ctx, http.MethodPost, path, nil, input, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) UpdateVendorFinding(ctx context.Context, vendorID, findingID string, input UpdateFindingInput) (*VendorFinding, error) {
	var out VendorFinding
	path := fmt.Sprintf("/vendors/%s/findings/%s", url.PathEscape(vendorID), url.PathEscape(findingID))
	if err := c.Do(ctx, http.MethodPatch, path, nil, input, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) DeleteVendorFinding(ctx context.Context, vendorID, findingID string) error {
	path := fmt.Sprintf("/vendors/%s/findings/%s", url.PathEscape(vendorID), url.PathEscape(findingID))
	return c.Do(ctx, http.MethodDelete, path, nil, nil, nil)
}

func (c *Client) ListVendorFindings(ctx context.Context, vendorID string) ([]VendorFinding, error) {
	path := fmt.Sprintf("/vendors/%s/findings", url.PathEscape(vendorID))
	return paginate[VendorFinding](ctx, c, path, nil)
}

// GetVendorFinding fetches a single finding. Vanta exposes no single-finding GET
// endpoint, so this lists the vendor's findings and filters by id.
func (c *Client) GetVendorFinding(ctx context.Context, vendorID, findingID string) (*VendorFinding, error) {
	findings, err := c.ListVendorFindings(ctx, vendorID)
	if err != nil {
		return nil, err
	}
	for i := range findings {
		if findings[i].ID == findingID {
			return &findings[i], nil
		}
	}
	return nil, &APIError{StatusCode: http.StatusNotFound, Message: fmt.Sprintf("finding %q not found on vendor %q", findingID, vendorID)}
}
