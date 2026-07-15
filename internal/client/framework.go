package client

import "context"

// Framework is a compliance framework with its progress counters.
type Framework struct {
	ID                   string  `json:"id"`
	DisplayName          string  `json:"displayName"`
	ShorthandName        string  `json:"shorthandName"`
	Description          string  `json:"description"`
	NumControlsCompleted float64 `json:"numControlsCompleted"`
	NumControlsTotal     float64 `json:"numControlsTotal"`
	NumTestsPassing      float64 `json:"numTestsPassing"`
	NumTestsTotal        float64 `json:"numTestsTotal"`
}

func (c *Client) ListFrameworks(ctx context.Context) ([]Framework, error) {
	return paginate[Framework](ctx, c, "/frameworks", nil)
}
