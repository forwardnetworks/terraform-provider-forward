// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package sdk

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Version represents the Forward Enterprise API version payload.
type Version struct {
	Build   string `json:"build"`
	Release string `json:"release"`
	Version string `json:"version"`
}

// GetVersion retrieves the Forward Enterprise API version information.
func (c *Client) GetVersion(ctx context.Context) (*Version, error) {
	if c == nil {
		return nil, fmt.Errorf("client is nil")
	}

	req, err := c.NewRequest(ctx, http.MethodGet, "/api/version", nil)
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
		return nil, fmt.Errorf("unexpected status %d retrieving version: %s", resp.StatusCode, string(body))
	}

	var payload Version
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode version response: %w", err)
	}

	return &payload, nil
}
