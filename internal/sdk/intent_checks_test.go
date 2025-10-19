package sdk

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_AddSnapshotCheck(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/snapshots/snap-1/checks" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		var payload NewCheckRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		if payload.Definition["checkType"] != "NQE" {
			t.Fatalf("unexpected definition: %#v", payload.Definition)
		}
		_ = json.NewEncoder(w).Encode(CheckResult{ID: "check-1", Status: "PASS"})
	}))
	defer server.Close()

	client, err := NewClient(context.Background(), Config{BaseURL: server.URL, APIKey: "token"})
	if err != nil {
		t.Fatalf("construct client: %v", err)
	}

	req := NewCheckRequest{
		Definition: CheckDefinition{"checkType": "NQE", "queryId": "FQ_test"},
	}
	result, err := client.AddSnapshotCheck(context.Background(), "snap-1", req, nil)
	if err != nil {
		t.Fatalf("AddSnapshotCheck returned error: %v", err)
	}
	if result == nil || result.ID != "check-1" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestClient_GetSnapshotCheck(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/snapshots/snap-1/checks/check-1" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(CheckResultWithDiagnosis{CheckResult: CheckResult{ID: "check-1"}})
	}))
	defer server.Close()

	client, err := NewClient(context.Background(), Config{BaseURL: server.URL, APIKey: "token"})
	if err != nil {
		t.Fatalf("construct client: %v", err)
	}

	result, err := client.GetSnapshotCheck(context.Background(), "snap-1", "check-1")
	if err != nil {
		t.Fatalf("GetSnapshotCheck returned error: %v", err)
	}
	if result == nil || result.ID != "check-1" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestClient_DeactivateSnapshotCheck(t *testing.T) {
	t.Parallel()

	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/api/snapshots/snap-1/checks/check-1" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodDelete {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, err := NewClient(context.Background(), Config{BaseURL: server.URL, APIKey: "token"})
	if err != nil {
		t.Fatalf("construct client: %v", err)
	}

	if err := client.DeactivateSnapshotCheck(context.Background(), "snap-1", "check-1"); err != nil {
		t.Fatalf("DeactivateSnapshotCheck returned error: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 call, got %d", calls)
	}
}

func TestClient_DeactivateSnapshotChecks(t *testing.T) {
	t.Parallel()

	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/api/snapshots/snap-1/checks" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodDelete {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, err := NewClient(context.Background(), Config{BaseURL: server.URL, APIKey: "token"})
	if err != nil {
		t.Fatalf("construct client: %v", err)
	}

	if err := client.DeactivateSnapshotChecks(context.Background(), "snap-1"); err != nil {
		t.Fatalf("DeactivateSnapshotChecks returned error: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 call, got %d", calls)
	}
}
