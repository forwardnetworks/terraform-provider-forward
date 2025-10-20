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

func TestCreateSnapshot(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/api/networks/net-1/snapshots" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		_ = json.NewEncoder(w).Encode(SnapshotDetails{Snapshot: Snapshot{ID: "snap-1", State: "PROCESSING", Note: payload["note"]}})
	}))
	defer server.Close()

	client, err := NewClient(context.Background(), Config{BaseURL: server.URL, APIKey: "token"})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	snapshot, err := client.CreateSnapshot(context.Background(), "net-1", SnapshotCreateRequest{Note: "test"})
	if err != nil {
		t.Fatalf("CreateSnapshot error: %v", err)
	}
	if snapshot.ID != "snap-1" || snapshot.Note != "test" {
		t.Fatalf("unexpected snapshot: %#v", snapshot)
	}
}

func TestGetSnapshot(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/api/networks/net-1/snapshots/snap-1" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(SnapshotDetails{Snapshot: Snapshot{ID: "snap-1", State: "PROCESSED"}})
	}))
	defer server.Close()

	client, err := NewClient(context.Background(), Config{BaseURL: server.URL, APIKey: "token"})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	snapshot, err := client.GetSnapshot(context.Background(), "net-1", "snap-1")
	if err != nil {
		t.Fatalf("GetSnapshot error: %v", err)
	}
	if snapshot.State != "PROCESSED" {
		t.Fatalf("unexpected snapshot state: %#v", snapshot)
	}
}

func TestDeleteSnapshot(t *testing.T) {
	t.Parallel()

	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != http.MethodDelete {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/api/snapshots/snap-1" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client, err := NewClient(context.Background(), Config{BaseURL: server.URL, APIKey: "token"})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	if err := client.DeleteSnapshot(context.Background(), "snap-1"); err != nil {
		t.Fatalf("DeleteSnapshot error: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 call, got %d", calls)
	}
}
