// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/internal/auth"
)

func TestClient_Search(t *testing.T) {
	// Mock server returning search results
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/data" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != "GET" {
			t.Errorf("unexpected method: %s", r.Method)
		}

		// Check query params
		query := r.URL.Query()
		if q := query.Get("query"); q != "test" {
			t.Errorf("expected query=test, got %s", q)
		}
		if st := query.Get("serviceType"); st != "OData" {
			t.Errorf("expected serviceType=OData, got %s", st)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"data": [
				{
					"uuid": "test-uuid",
					"name": "TestService",
					"version": "1.0.0",
					"serviceType": "OData",
					"environment": {"name": "Test", "type": "Test"},
					"application": {"name": "TestApp"}
				}
			],
			"totalResults": 1,
			"limit": 20,
			"offset": 0
		}`))
	}))
	defer server.Close()

	// Create client with mock HTTP client (no auth)
	client := &Client{
		httpClient: server.Client(),
		baseURL:    server.URL,
	}

	// Execute search
	ctx := context.Background()
	opts := SearchOptions{
		Query:       "test",
		ServiceType: "OData",
		Limit:       20,
	}
	resp, err := client.Search(ctx, opts)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}

	if resp.TotalResults != 1 {
		t.Errorf("expected 1 result, got %d", resp.TotalResults)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 data item, got %d", len(resp.Data))
	}
	if resp.Data[0].Name != "TestService" {
		t.Errorf("unexpected service name: %s", resp.Data[0].Name)
	}
}

func TestClient_Search_EmptyQuery(t *testing.T) {
	// Mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check that query param is absent when empty
		query := r.URL.Query()
		if _, exists := query["query"]; exists {
			t.Error("expected query param to be absent when empty")
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data": [], "totalResults": 0, "limit": 20, "offset": 0}`))
	}))
	defer server.Close()

	client := &Client{
		httpClient: server.Client(),
		baseURL:    server.URL,
	}

	ctx := context.Background()
	opts := SearchOptions{} // Empty query
	resp, err := client.Search(ctx, opts)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}

	if resp.TotalResults != 0 {
		t.Errorf("expected 0 results, got %d", resp.TotalResults)
	}
}

func TestClient_Search_HTTPError(t *testing.T) {
	// Mock server returning error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := &Client{
		httpClient: server.Client(),
		baseURL:    server.URL,
	}

	ctx := context.Background()
	opts := SearchOptions{Query: "test"}
	_, err := client.Search(ctx, opts)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestNewClient_NoCredential(t *testing.T) {
	ctx := context.Background()
	_, err := NewClient(ctx, "nonexistent-profile")
	if err == nil {
		t.Fatal("expected error when no credential found")
	}
	if _, ok := err.(*auth.ErrNoCredential); !ok {
		t.Errorf("expected ErrNoCredential, got %T: %v", err, err)
	}
}
