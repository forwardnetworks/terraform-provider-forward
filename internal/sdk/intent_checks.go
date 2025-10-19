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

// CheckDefinition represents the underlying definition payload for an intent check.
type CheckDefinition map[string]any

// NewCheckRequest models the payload to create a new check.
type NewCheckRequest struct {
	Definition            CheckDefinition `json:"definition"`
	Enabled               *bool           `json:"enabled,omitempty"`
	Name                  string          `json:"name,omitempty"`
	Note                  string          `json:"note,omitempty"`
	PerfMonitoringEnabled *bool           `json:"perfMonitoringEnabled,omitempty"`
	Priority              string          `json:"priority,omitempty"`
	Tags                  []string        `json:"tags,omitempty"`
}

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

// CheckResultWithDiagnosis includes diagnosis metadata for a single check lookup.
type CheckResultWithDiagnosis struct {
	CheckResult
	Diagnosis *CheckDiagnosis `json:"diagnosis"`
}

// CheckDiagnosis captures the violation/details for a failed check.
type CheckDiagnosis struct {
	Summary           string            `json:"summary"`
	Details           []DiagnosisDetail `json:"details"`
	DetailsIncomplete *bool             `json:"detailsIncomplete"`
}

// DiagnosisDetail enumerates detailed findings for a violation.
type DiagnosisDetail struct {
	Query      string               `json:"query"`
	References []DiagnosisReference `json:"references"`
}

// DiagnosisReference links a diagnosis detail to specific devices/files.
type DiagnosisReference struct {
	Key   string                 `json:"key"`
	Value string                 `json:"value"`
	Files map[string][]LineRange `json:"files"`
}

// LineRange indicates a span within a device file.
type LineRange struct {
	Start *int32 `json:"start,omitempty"`
	End   *int32 `json:"end,omitempty"`
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

// AddSnapshotCheck creates a new intent check for the specified snapshot.
func (c *Client) AddSnapshotCheck(ctx context.Context, snapshotID string, reqBody NewCheckRequest, persistent *bool) (*CheckResult, error) {
	if c == nil {
		return nil, fmt.Errorf("client is nil")
	}

	snapshotID = strings.TrimSpace(snapshotID)
	if snapshotID == "" {
		return nil, fmt.Errorf("snapshotID must be provided")
	}

	if reqBody.Definition == nil {
		return nil, fmt.Errorf("definition must be provided")
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal check payload: %w", err)
	}

	path := fmt.Sprintf("/api/snapshots/%s/checks", url.PathEscape(snapshotID))
	if persistent != nil {
		params := url.Values{}
		params.Set("persistent", fmt.Sprintf("%t", *persistent))
		path = path + "?" + params.Encode()
	}

	req, err := c.NewRequest(ctx, http.MethodPost, path, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}

	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("create check request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<14))
		return nil, fmt.Errorf("unexpected status %d creating check: %s", resp.StatusCode, string(body))
	}

	var result CheckResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode create check response: %w", err)
	}

	return &result, nil
}

// GetSnapshotCheck retrieves a specific check by ID for the given snapshot.
func (c *Client) GetSnapshotCheck(ctx context.Context, snapshotID, checkID string) (*CheckResultWithDiagnosis, error) {
	if c == nil {
		return nil, fmt.Errorf("client is nil")
	}

	snapshotID = strings.TrimSpace(snapshotID)
	checkID = strings.TrimSpace(checkID)
	if snapshotID == "" || checkID == "" {
		return nil, fmt.Errorf("snapshotID and checkID must be provided")
	}

	path := fmt.Sprintf("/api/snapshots/%s/checks/%s", url.PathEscape(snapshotID), url.PathEscape(checkID))
	req, err := c.NewRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("retrieve check request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<14))
		return nil, fmt.Errorf("unexpected status %d retrieving check: %s", resp.StatusCode, string(body))
	}

	var result CheckResultWithDiagnosis
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode check response: %w", err)
	}

	return &result, nil
}

// DeactivateSnapshotCheck disables a specific check for a snapshot.
func (c *Client) DeactivateSnapshotCheck(ctx context.Context, snapshotID, checkID string) error {
	if c == nil {
		return fmt.Errorf("client is nil")
	}

	snapshotID = strings.TrimSpace(snapshotID)
	checkID = strings.TrimSpace(checkID)
	if snapshotID == "" || checkID == "" {
		return fmt.Errorf("snapshotID and checkID must be provided")
	}

	path := fmt.Sprintf("/api/snapshots/%s/checks/%s", url.PathEscape(snapshotID), url.PathEscape(checkID))
	req, err := c.NewRequest(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return err
	}

	resp, err := c.Do(req)
	if err != nil {
		return fmt.Errorf("deactivate check request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<14))
		return fmt.Errorf("unexpected status %d deactivating check: %s", resp.StatusCode, string(body))
	}

	return nil
}

// DeactivateSnapshotChecks disables all checks for a snapshot.
func (c *Client) DeactivateSnapshotChecks(ctx context.Context, snapshotID string) error {
	if c == nil {
		return fmt.Errorf("client is nil")
	}

	snapshotID = strings.TrimSpace(snapshotID)
	if snapshotID == "" {
		return fmt.Errorf("snapshotID must be provided")
	}

	path := fmt.Sprintf("/api/snapshots/%s/checks", url.PathEscape(snapshotID))
	req, err := c.NewRequest(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return err
	}

	resp, err := c.Do(req)
	if err != nil {
		return fmt.Errorf("deactivate checks request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<14))
		return fmt.Errorf("unexpected status %d deactivating checks: %s", resp.StatusCode, string(body))
	}

	return nil
}
