package client

import (
	"context"
	"net/url"
)

// Person is the subset of Vanta's person object exposed by the people data
// source.
type Person struct {
	ID           string `json:"id"`
	EmailAddress string `json:"emailAddress"`
	Name         struct {
		First   *string `json:"first"`
		Last    *string `json:"last"`
		Display string  `json:"display"`
	} `json:"name"`
	Employment struct {
		Status   string  `json:"status"`
		JobTitle *string `json:"jobTitle"`
	} `json:"employment"`
}

// ListPeopleFilter narrows the people list. Zero values are omitted.
type ListPeopleFilter struct {
	EmploymentStatus string
}

func (c *Client) ListPeople(ctx context.Context, filter ListPeopleFilter) ([]Person, error) {
	q := url.Values{}
	if filter.EmploymentStatus != "" {
		q.Set("employmentStatus", filter.EmploymentStatus)
	}
	return paginate[Person](ctx, c, "/people", q)
}
