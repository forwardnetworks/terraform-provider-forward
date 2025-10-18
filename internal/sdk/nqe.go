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
	"strings"
)

// NqeQueryRequest captures the body parameters for executing an NQE query.
type NqeQueryRequest struct {
	Query        *string          `json:"query,omitempty"`
	QueryID      *string          `json:"queryId,omitempty"`
	CommitID     *string          `json:"commitId,omitempty"`
	Parameters   map[string]any   `json:"parameters,omitempty"`
	QueryOptions *NqeQueryOptions `json:"queryOptions,omitempty"`
}

// NqeQueryOptions allows limiting returned rows.
type NqeQueryOptions struct {
	Limit  *int `json:"limit,omitempty"`
	Offset *int `json:"offset,omitempty"`
}

// NqeRunResult represents the successful execution payload.
type NqeRunResult struct {
	SnapshotID    string            `json:"snapshotId"`
	Items         []json.RawMessage `json:"items"`
	TotalNumItems *int64            `json:"totalNumItems"`
}

// RunNQEQuery executes an NQE query against the specified network or snapshot.
func (c *Client) RunNQEQuery(ctx context.Context, networkID, snapshotID string, reqBody NqeQueryRequest) (*NqeRunResult, error) {
	if c == nil {
		return nil, fmt.Errorf("client is nil")
	}

	if reqBody.Query == nil && reqBody.QueryID == nil {
		return nil, fmt.Errorf("either query or query_id must be provided")
	}

	if reqBody.Parameters == nil {
		reqBody.Parameters = map[string]any{}
	}

	queryParams := url.Values{}
	if snapshotID != "" {
		queryParams.Set("snapshotId", snapshotID)
	}
	if networkID != "" {
		queryParams.Set("networkId", networkID)
	}

	if snapshotID == "" && networkID == "" {
		return nil, fmt.Errorf("either snapshotID or networkID must be supplied")
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal nqe request: %w", err)
	}

	path := "/api/nqe"
	if encoded := queryParams.Encode(); encoded != "" {
		path = path + "?" + encoded
	}

	req, err := c.NewRequest(ctx, http.MethodPost, path, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}

	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute NQE request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<14))
		return nil, fmt.Errorf("unexpected status %d running NQE query: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var result NqeRunResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode NQE response: %w", err)
	}

	return &result, nil
}
