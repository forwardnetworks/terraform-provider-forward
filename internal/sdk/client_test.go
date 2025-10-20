// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// Copyright (c) HashiCorp, Inc.

package sdk

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClient_DoRetriesOnServerError(t *testing.T) {
	t.Parallel()

	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, err := NewClient(context.Background(), Config{
		BaseURL:    server.URL,
		APIKey:     "token",
		MaxRetries: 5,
		RetryDelay: 5 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("construct client: %v", err)
	}

	req, err := client.NewRequest(context.Background(), http.MethodGet, "/test", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("expected success after retries, got error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}
	if attempts != 3 {
		t.Fatalf("expected 3 attempts, got %d", attempts)
	}
}

func TestClient_DoStopsAfterMaxRetries(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client, err := NewClient(context.Background(), Config{
		BaseURL:    server.URL,
		APIKey:     "token",
		MaxRetries: 2,
		RetryDelay: 1 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("construct client: %v", err)
	}

	req, err := client.NewRequest(context.Background(), http.MethodGet, "/test", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	_, err = client.Do(req)
	if err == nil {
		t.Fatalf("expected error after retries")
	}
}

func TestClient_DoRespectsContextCancel(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client, err := NewClient(context.Background(), Config{
		BaseURL:    server.URL,
		APIKey:     "token",
		MaxRetries: 5,
		RetryDelay: 50 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("construct client: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	req, err := client.NewRequest(ctx, http.MethodGet, "/test", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	cancel()

	_, err = client.Do(req)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation error, got %v", err)
	}
}
