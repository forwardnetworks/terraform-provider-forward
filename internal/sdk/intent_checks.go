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
	"strings"
)

// CheckResult represents the outcome of a Forward Enterprise intent check execution.
type CheckResult struct {
	ID                    string          `json:"id"`
	Name                  string          `json:"name"`
	Status                string          `json:"status"`
	Priority              string          `json:"priority"`
	Description           string          `json:"description"`
	Note                  string          `json:"note"`
	Creator               string          `json:"creator"`
	CreatorID             string          `json:"creatorId"`
	Editor                string          `json:"editor"`
	EditorID              string          `json:"editorId"`
	Enabled               *bool           `json:"enabled"`
	PerfMonitoringEnabled *bool           `json:"perfMonitoringEnabled"`
	Tags                  []string        `json:"tags"`
	NumViolations         *int64          `json:"numViolations"`
	CreationDateMillis    *int64          `json:"creationDateMillis"`
	DefinitionDateMillis  *int64          `json:"definitionDateMillis"`
	EditDateMillis        *int64          `json:"editDateMillis"`
	ExecutionDateMillis   *int64          `json:"executionDateMillis"`
	ExecutionDuration     *int64          `json:"executionDurationMillis"`
	Definition            json.RawMessage `json:"definition"`
}

// CheckListOptions controls filtering when listing intent checks.
type CheckListOptions struct {
	Types      []string
	Statuses   []string
	Priorities []string
}

// ListSnapshotChecks retrieves check results for the specified snapshot.
func (c *Client) ListSnapshotChecks(ctx context.Context, snapshotID string, opts CheckListOptions) ([]CheckResult, error) {
	if c == nil {
		return nil, fmt.Errorf("client is nil")
	}

	snapshotID = strings.TrimSpace(snapshotID)
	if snapshotID == "" {
		return nil, fmt.Errorf("snapshotID must be provided")
	}

	escapedID := url.PathEscape(snapshotID)
	path := fmt.Sprintf("/api/snapshots/%s/checks", escapedID)

	query := url.Values{}
	for _, status := range opts.Statuses {
		status = strings.TrimSpace(status)
		if status != "" {
			query.Add("status", status)
		}
	}
	for _, priority := range opts.Priorities {
		priority = strings.TrimSpace(priority)
		if priority != "" {
			query.Add("priority", priority)
		}
	}
	for _, checkType := range opts.Types {
		checkType = strings.TrimSpace(checkType)
		if checkType != "" {
			query.Add("type", checkType)
		}
	}

	if encoded := query.Encode(); encoded != "" {
		path = path + "?" + encoded
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

	switch resp.StatusCode {
	case http.StatusOK:
		// continue
	default:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<14))
		return nil, fmt.Errorf("unexpected status %d retrieving checks: %s", resp.StatusCode, string(body))
	}

	var checks []CheckResult
	if err := json.NewDecoder(resp.Body).Decode(&checks); err != nil {
		return nil, fmt.Errorf("decode checks response: %w", err)
	}

	return checks, nil
}
