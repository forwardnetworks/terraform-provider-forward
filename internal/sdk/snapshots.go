// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package sdk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// Snapshot describes a network snapshot returned by the Forward Enterprise API.
type Snapshot struct {
	ID                 string `json:"id"`
	State              string `json:"state"`
	ProcessingTrigger  string `json:"processingTrigger"`
	ParentSnapshotID   string `json:"parentSnapshotId"`
	Note               string `json:"note"`
	FavoritedBy        string `json:"favoritedBy"`
	FavoritedByUserID  string `json:"favoritedByUserId"`
	CreationDateMillis *int64 `json:"creationDateMillis"`
	ProcessedAtMillis  *int64 `json:"processedAtMillis"`
	RestoredAtMillis   *int64 `json:"restoredAtMillis"`
	FavoritedAtMillis  *int64 `json:"favoritedAtMillis"`
	IsDraft            *bool  `json:"isDraft"`
}

// SnapshotListOptions controls the ListSnapshots behavior.
type SnapshotListOptions struct {
	Limit           *int
	IncludeArchived *bool
}

// ListSnapshots retrieves snapshots for the supplied network identifier.
func (c *Client) ListSnapshots(ctx context.Context, networkID string, opts SnapshotListOptions) ([]Snapshot, error) {
	if c == nil {
		return nil, fmt.Errorf("client is nil")
	}

	networkID = strings.TrimSpace(networkID)
	if networkID == "" {
		return nil, fmt.Errorf("networkID must be provided")
	}

	escapedNetworkID := url.PathEscape(networkID)
	path := fmt.Sprintf("/api/networks/%s/snapshots", escapedNetworkID)

	query := url.Values{}
	if opts.Limit != nil {
		query.Set("limit", strconv.Itoa(*opts.Limit))
	}

	if opts.IncludeArchived != nil {
		query.Set("includeArchived", strconv.FormatBool(*opts.IncludeArchived))
	}

	if enc := query.Encode(); enc != "" {
		path = path + "?" + enc
	}

	req, err := c.NewRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<14))
		return nil, fmt.Errorf("unexpected status %d retrieving snapshots: %s", resp.StatusCode, string(body))
	}

	var payload struct {
		Snapshots []Snapshot `json:"snapshots"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode snapshots response: %w", err)
	}

	return payload.Snapshots, nil
}

// SnapshotCreateRequest represents optional parameters when creating a snapshot.
type SnapshotCreateRequest struct {
	Note string `json:"note,omitempty"`
}

// SnapshotDetails represents detailed snapshot information.
type SnapshotDetails struct {
	Snapshot
}

// CreateSnapshot initiates a new snapshot collection for the given network.
func (c *Client) CreateSnapshot(ctx context.Context, networkID string, reqBody SnapshotCreateRequest) (*SnapshotDetails, error) {
	if c == nil {
		return nil, fmt.Errorf("client is nil")
	}

	networkID = strings.TrimSpace(networkID)
	if networkID == "" {
		return nil, fmt.Errorf("networkID must be provided")
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal snapshot request: %w", err)
	}

	reader := bytes.NewReader(body)
	path := fmt.Sprintf("/api/networks/%s/snapshots", url.PathEscape(networkID))
	req, err := c.NewRequest(ctx, http.MethodPost, path, reader)
	if err != nil {
		return nil, err
	}

	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute snapshot create request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<14))
		return nil, fmt.Errorf("unexpected status %d creating snapshot: %s", resp.StatusCode, string(body))
	}

	var snapshot SnapshotDetails
	if err := json.NewDecoder(resp.Body).Decode(&snapshot); err != nil {
		return nil, fmt.Errorf("decode snapshot create response: %w", err)
	}

	return &snapshot, nil
}

// GetSnapshot retrieves a snapshot by ID for the provided network.
func (c *Client) GetSnapshot(ctx context.Context, networkID, snapshotID string) (*SnapshotDetails, error) {
	if c == nil {
		return nil, fmt.Errorf("client is nil")
	}

	networkID = strings.TrimSpace(networkID)
	snapshotID = strings.TrimSpace(snapshotID)
	if networkID == "" || snapshotID == "" {
		return nil, fmt.Errorf("networkID and snapshotID must be provided")
	}

	path := fmt.Sprintf("/api/networks/%s/snapshots/%s", url.PathEscape(networkID), url.PathEscape(snapshotID))
	req, err := c.NewRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute snapshot get request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("snapshot %s not found", snapshotID)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<14))
		return nil, fmt.Errorf("unexpected status %d retrieving snapshot: %s", resp.StatusCode, string(body))
	}

	var snapshot SnapshotDetails
	if err := json.NewDecoder(resp.Body).Decode(&snapshot); err != nil {
		return nil, fmt.Errorf("decode snapshot response: %w", err)
	}

	return &snapshot, nil
}

// DeleteSnapshot removes a snapshot by ID.
func (c *Client) DeleteSnapshot(ctx context.Context, snapshotID string) error {
	if c == nil {
		return fmt.Errorf("client is nil")
	}

	snapshotID = strings.TrimSpace(snapshotID)
	if snapshotID == "" {
		return fmt.Errorf("snapshotID must be provided")
	}

	path := fmt.Sprintf("/api/snapshots/%s", url.PathEscape(snapshotID))
	req, err := c.NewRequest(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return err
	}

	resp, err := c.Do(req)
	if err != nil {
		return fmt.Errorf("execute snapshot delete request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<14))
		return fmt.Errorf("unexpected status %d deleting snapshot: %s", resp.StatusCode, string(body))
	}

	return nil
}
