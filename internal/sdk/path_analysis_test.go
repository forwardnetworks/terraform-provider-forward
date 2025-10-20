// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// Copyright (c) HashiCorp, Inc.

package sdk

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSearchPaths(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/networks/net-1/paths" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("dstIp") != "10.0.0.1" {
			t.Fatalf("missing dstIp")
		}
		_ = json.NewEncoder(w).Encode(PathSearchResult{
			SrcIPLocationType: "INTERFACE",
			DstIPLocationType: "INTERFACE",
			Info:              PathCollection{Paths: []Path{{ForwardingOutcome: "DELIVERED"}}},
		})
	}))
	defer server.Close()

	client, err := NewClient(context.Background(), Config{BaseURL: server.URL, APIKey: "token"})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	result, err := client.SearchPaths(context.Background(), "net-1", PathSearchParams{SrcIP: "10.0.0.2", DstIP: "10.0.0.1"})
	if err != nil {
		t.Fatalf("SearchPaths error: %v", err)
	}
	if len(result.Info.Paths) != 1 {
		t.Fatalf("unexpected result: %#v", result)
	}
}
