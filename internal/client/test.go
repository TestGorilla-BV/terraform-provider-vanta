package client

import (
	"context"
	"net/url"
)

// Test is the subset of a Vanta test exposed by the tests data source.
type Test struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Status          string `json:"status"`
	Category        string `json:"category"`
	LastTestRunDate string `json:"lastTestRunDate"`
}

// ListTestsFilter narrows the tests list. Zero values are omitted.
type ListTestsFilter struct {
	StatusFilter      string
	FrameworkFilter   string
	IntegrationFilter string
}

func (c *Client) ListTests(ctx context.Context, filter ListTestsFilter) ([]Test, error) {
	q := url.Values{}
	if filter.StatusFilter != "" {
		q.Set("statusFilter", filter.StatusFilter)
	}
	if filter.FrameworkFilter != "" {
		q.Set("frameworkFilter", filter.FrameworkFilter)
	}
	if filter.IntegrationFilter != "" {
		q.Set("integrationFilter", filter.IntegrationFilter)
	}
	return paginate[Test](ctx, c, "/tests", q)
}
