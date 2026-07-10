// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/JordtenBulte-OLC/mxcli/internal/auth"
)

const baseURL = "https://catalog.mendix.com/rest/search/v5"

// Client wraps the Catalog Search API with authentication.
type Client struct {
	httpClient *http.Client
	baseURL    string
}

// NewClient creates a new Catalog API client using the specified auth profile.
// The profile is resolved via internal/auth (env vars or ~/.mxcli/auth.json).
func NewClient(ctx context.Context, profile string) (*Client, error) {
	httpClient, err := auth.ClientFor(ctx, profile)
	if err != nil {
		return nil, err
	}
	return &Client{
		httpClient: httpClient,
		baseURL:    baseURL,
	}, nil
}

// Search executes a catalog search with the given options.
// Calls GET /data with query parameters and returns parsed results.
func (c *Client) Search(ctx context.Context, opts SearchOptions) (*SearchResponse, error) {
	// Build query params
	params := url.Values{}
	if opts.Query != "" {
		params.Set("query", opts.Query)
	}
	if opts.ServiceType != "" {
		params.Set("serviceType", opts.ServiceType)
	}
	if opts.ProductionEndpointsOnly {
		params.Set("productionEndpointsOnly", "true")
	}
	if opts.OwnedContentOnly {
		params.Set("ownedContentOnly", "true")
	}
	if opts.Limit > 0 {
		params.Set("limit", strconv.Itoa(opts.Limit))
	}
	if opts.Offset > 0 {
		params.Set("offset", strconv.Itoa(opts.Offset))
	}

	// Make request
	reqURL := c.baseURL + "/data?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		// auth.authTransport wraps 401/403 as auth.ErrUnauthenticated
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("catalog API returned status %d", resp.StatusCode)
	}

	var result SearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &result, nil
}

// GetEndpoint retrieves detailed endpoint metadata by UUID.
// Calls GET /endpoints/{uuid} and returns parsed result including embedded contract.
func (c *Client) GetEndpoint(ctx context.Context, uuid string) (*EndpointDetails, error) {
	reqURL := c.baseURL + "/endpoints/" + uuid
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		// auth.authTransport wraps 401/403 as auth.ErrUnauthenticated
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("endpoint %s not found", uuid)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("catalog API returned status %d", resp.StatusCode)
	}

	var result EndpointDetails
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &result, nil
}
