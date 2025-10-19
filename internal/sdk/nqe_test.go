package sdk

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_ListNQEQueries(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/nqe/queries" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.RawQuery != "" {
			t.Fatalf("unexpected query string: %s", r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode([]NqeQuery{{
			QueryID:    "FQ_test",
			Repository: "ORG",
			Path:       "/L3/Example",
			Intent:     "Validate MTU",
		}})
	}))
	defer server.Close()

	client, err := NewClient(context.Background(), Config{BaseURL: server.URL, APIKey: "token"})
	if err != nil {
		t.Fatalf("construct client: %v", err)
	}

	queries, err := client.ListNQEQueries(context.Background(), "")
	if err != nil {
		t.Fatalf("ListNQEQueries returned error: %v", err)
	}
	if len(queries) != 1 || queries[0].QueryID != "FQ_test" {
		t.Fatalf("unexpected queries: %#v", queries)
	}
}

func TestClient_RunNQEDiff(t *testing.T) {
	t.Parallel()

	var received NqeDiffRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/nqe-diffs/before/after" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		_ = json.NewEncoder(w).Encode(NqeDiffResult{Rows: []NqeDiffEntry{{Type: "ADDED"}}})
	}))
	defer server.Close()

	client, err := NewClient(context.Background(), Config{BaseURL: server.URL, APIKey: "token"})
	if err != nil {
		t.Fatalf("construct client: %v", err)
	}

	req := NqeDiffRequest{QueryID: "FQ_test"}
	result, err := client.RunNQEDiff(context.Background(), "before", "after", req)
	if err != nil {
		t.Fatalf("RunNQEDiff returned error: %v", err)
	}
	if result == nil || len(result.Rows) != 1 {
		t.Fatalf("unexpected diff result: %#v", result)
	}
	if received.QueryID != "FQ_test" {
		t.Fatalf("unexpected request payload: %#v", received)
	}
}
