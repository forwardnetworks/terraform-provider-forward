// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package sdk

import (
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
