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
	Limit         *int           `json:"limit,omitempty"`
	Offset        *int           `json:"offset,omitempty"`
	SortBy        *SortOrder     `json:"sortBy,omitempty"`
	ColumnFilters []ColumnFilter `json:"columnFilters,omitempty"`
}

// NqeRunResult represents the successful execution payload.
type NqeRunResult struct {
	SnapshotID    string            `json:"snapshotId"`
	Items         []json.RawMessage `json:"items"`
	TotalNumItems *int64            `json:"totalNumItems"`
}

// NqeQuery describes a stored query from the Forward Enterprise NQE library.
type NqeQuery struct {
	QueryID    string `json:"queryId"`
	Repository string `json:"repository"`
	Path       string `json:"path"`
	Intent     string `json:"intent"`
}

// SortOrder describes how NQE results should be ordered.
type SortOrder struct {
	ColumnName string `json:"columnName"`
	Order      string `json:"order,omitempty"` // ASC or DESC
}

// ColumnFilter represents a filter applied to NQE query results.
type ColumnFilter struct {
	ColumnName string `json:"columnName"`
	Operator   string `json:"operator"`
	Value      string `json:"value,omitempty"`
	LowerBound string `json:"lowerBound,omitempty"`
	UpperBound string `json:"upperBound,omitempty"`
}

// NqeDiffRequest captures the inputs for diffing two snapshots with an NQE query.
type NqeDiffRequest struct {
	QueryID    string           `json:"queryId"`
	CommitID   *string          `json:"commitId,omitempty"`
	Options    *NqeQueryOptions `json:"options,omitempty"`
	Parameters map[string]any   `json:"parameters,omitempty"`
}

// NqeDiffResult represents the diff output between two snapshot executions.
type NqeDiffResult struct {
	Rows         []NqeDiffEntry `json:"rows"`
	TotalNumRows *int32         `json:"totalNumRows"`
}

// NqeDiffEntry captures an individual diff row.
type NqeDiffEntry struct {
	Type   string     `json:"type"`
	Before *NqeRecord `json:"before,omitempty"`
	After  *NqeRecord `json:"after,omitempty"`
}

// NqeRecord represents a single NQE row.
type NqeRecord struct {
	Fields map[string]json.RawMessage `json:"fields"`
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

// ListNQEQueries retrieves committed NQE queries, optionally filtered by directory.
func (c *Client) ListNQEQueries(ctx context.Context, dir string) ([]NqeQuery, error) {
	if c == nil {
		return nil, fmt.Errorf("client is nil")
	}

	path := "/api/nqe/queries"
	if strings.TrimSpace(dir) != "" {
		params := url.Values{}
		params.Set("dir", dir)
		path = path + "?" + params.Encode()
	}

	req, err := c.NewRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("list NQE queries request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<14))
		return nil, fmt.Errorf("unexpected status %d listing NQE queries: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var queries []NqeQuery
	if err := json.NewDecoder(resp.Body).Decode(&queries); err != nil {
		return nil, fmt.Errorf("decode NQE query list: %w", err)
	}

	return queries, nil
}

// RunNQEDiff executes an NQE diff between two snapshot IDs.
func (c *Client) RunNQEDiff(ctx context.Context, beforeSnapshotID, afterSnapshotID string, reqBody NqeDiffRequest) (*NqeDiffResult, error) {
	if c == nil {
		return nil, fmt.Errorf("client is nil")
	}

	before := strings.TrimSpace(beforeSnapshotID)
	after := strings.TrimSpace(afterSnapshotID)
	if before == "" || after == "" {
		return nil, fmt.Errorf("beforeSnapshotID and afterSnapshotID must be provided")
	}

	if reqBody.QueryID == "" {
		return nil, fmt.Errorf("queryId must be provided")
	}

	if reqBody.Parameters == nil {
		reqBody.Parameters = map[string]any{}
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal NQE diff request: %w", err)
	}

	path := fmt.Sprintf("/api/nqe-diffs/%s/%s", url.PathEscape(before), url.PathEscape(after))

	req, err := c.NewRequest(ctx, http.MethodPost, path, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}

	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute NQE diff request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<14))
		return nil, fmt.Errorf("unexpected status %d running NQE diff: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var result NqeDiffResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode NQE diff response: %w", err)
	}

	return &result, nil
}
