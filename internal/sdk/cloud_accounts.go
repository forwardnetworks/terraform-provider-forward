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

// CloudAccount describes a Forward cloud account setup.
type CloudAccount struct {
	Type                          string              `json:"type"`
	Name                          string              `json:"name"`
	Collect                       bool                `json:"collect"`
	ProxyServerID                 string              `json:"proxyServerId,omitempty"`
	Regions                       map[string]Region   `json:"regions,omitempty"`
	RegionToProxyServerID         map[string]string   `json:"regionToProxyServerId"`
	AssumeRoleInfos               []AWSAssumeRoleInfo `json:"assumeRoleInfos,omitempty"`
	UseForwardAccountToAssumeRole *bool               `json:"useForwardAccountToAssumeRole,omitempty"`
	Concurrency                   *int64              `json:"concurrency,omitempty"`
	ConnectionTimeoutSeconds      *int64              `json:"connectionTimeoutSeconds,omitempty"`
	RequestTimeoutSeconds         *int64              `json:"requestTimeoutSeconds,omitempty"`
}

// Region captures the last successful test timestamp returned by Forward.
type Region struct {
	TestInstant int64 `json:"testInstant,omitempty"`
}

// AWSAssumeRoleInfo describes one AWS account and the role Forward should assume.
type AWSAssumeRoleInfo struct {
	AccountID   string `json:"accountId"`
	AccountName string `json:"accountName,omitempty"`
	RoleArn     string `json:"roleArn,omitempty"`
	ExternalID  string `json:"externalId,omitempty"`
	Enabled     bool   `json:"enabled"`
	ErrorMsg    string `json:"errorMsg,omitempty"`
}

// AWSCloudAccountRequest is the create/update payload for Forward AWS setups.
type AWSCloudAccountRequest struct {
	Type                          string              `json:"type"`
	Name                          string              `json:"name,omitempty"`
	Collect                       *bool               `json:"collect,omitempty"`
	Username                      string              `json:"username,omitempty"`
	Password                      string              `json:"password,omitempty"`
	ProxyServerID                 *string             `json:"proxyServerId,omitempty"`
	Regions                       map[string]int64    `json:"regions,omitempty"`
	RegionToProxyServerID         map[string]string   `json:"regionToProxyServerId,omitempty"`
	AssumeRoleInfos               []AWSAssumeRoleInfo `json:"assumeRoleInfos,omitempty"`
	UseForwardAccountToAssumeRole *bool               `json:"useForwardAccountToAssumeRole,omitempty"`
	Concurrency                   *int64              `json:"concurrency,omitempty"`
	ConnectionTimeoutSeconds      *int64              `json:"connectionTimeoutSeconds,omitempty"`
	RequestTimeoutSeconds         *int64              `json:"requestTimeoutSeconds,omitempty"`
}

// AWSCloudAccountCredentialRequest updates stored AWS collector credentials for a Forward setup.
type AWSCloudAccountCredentialRequest struct {
	Type     string `json:"type"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

// AWSAssumeRoleExternalIDResponse is returned by Forward for assume-role setups.
type AWSAssumeRoleExternalIDResponse struct {
	ExternalID string `json:"externalId"`
}

// ListCloudAccounts retrieves cloud account setups for a Forward network.
func (c *Client) ListCloudAccounts(ctx context.Context, networkID string) ([]CloudAccount, error) {
	if c == nil {
		return nil, fmt.Errorf("client is nil")
	}
	networkID = strings.TrimSpace(networkID)
	if networkID == "" {
		return nil, fmt.Errorf("networkID must be provided")
	}

	path := fmt.Sprintf("/api/networks/%s/cloudAccounts", url.PathEscape(networkID))
	req, err := c.NewRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute cloud accounts list request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, unexpectedStatus(resp, "listing cloud accounts")
	}

	var accounts []CloudAccount
	if err := json.NewDecoder(resp.Body).Decode(&accounts); err != nil {
		return nil, fmt.Errorf("decode cloud accounts response: %w", err)
	}

	return accounts, nil
}

// GetCloudAccount retrieves one cloud account setup by name.
func (c *Client) GetCloudAccount(ctx context.Context, networkID, name string) (*CloudAccount, error) {
	accounts, err := c.ListCloudAccounts(ctx, networkID)
	if err != nil {
		return nil, err
	}

	name = strings.TrimSpace(name)
	for _, account := range accounts {
		if account.Name == name {
			return &account, nil
		}
	}

	return nil, fmt.Errorf("cloud account %q not found", name)
}

// AWSAssumeRoleExternalID retrieves the Forward-generated external ID for AWS assume-role collection.
func (c *Client) AWSAssumeRoleExternalID(ctx context.Context, networkID string) (string, error) {
	if c == nil {
		return "", fmt.Errorf("client is nil")
	}
	networkID = strings.TrimSpace(networkID)
	if networkID == "" {
		return "", fmt.Errorf("networkID must be provided")
	}

	path := fmt.Sprintf("/api/networks/%s/cloudAccounts/aws/assumeRole/externalId", url.PathEscape(networkID))
	req, err := c.NewRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return "", err
	}

	resp, err := c.Do(req)
	if err != nil {
		return "", fmt.Errorf("execute AWS external ID request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", unexpectedStatus(resp, "retrieving AWS assume-role external ID")
	}

	var payload AWSAssumeRoleExternalIDResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", fmt.Errorf("decode AWS external ID response: %w", err)
	}

	return strings.TrimSpace(payload.ExternalID), nil
}

// CreateCloudAccount creates a Forward cloud account setup.
func (c *Client) CreateCloudAccount(ctx context.Context, networkID string, body AWSCloudAccountRequest) error {
	return c.writeCloudAccount(ctx, http.MethodPost, networkID, "", body)
}

// UpdateCloudAccount patches a Forward cloud account setup by name.
func (c *Client) UpdateCloudAccount(ctx context.Context, networkID, name string, body AWSCloudAccountRequest) error {
	return c.writeCloudAccount(ctx, http.MethodPatch, networkID, name, body)
}

// UpdateCloudAccountCredential updates stored credentials for a Forward cloud account setup.
func (c *Client) UpdateCloudAccountCredential(ctx context.Context, networkID, name string, body AWSCloudAccountCredentialRequest) error {
	if c == nil {
		return fmt.Errorf("client is nil")
	}
	networkID = strings.TrimSpace(networkID)
	name = strings.TrimSpace(name)
	if networkID == "" || name == "" {
		return fmt.Errorf("networkID and cloud account name must be provided")
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal cloud account credential request: %w", err)
	}

	path := fmt.Sprintf("/api/networks/%s/cloudAccounts/%s/credential", url.PathEscape(networkID), url.PathEscape(name))
	req, err := c.NewRequest(ctx, http.MethodPost, path, bytes.NewReader(payload))
	if err != nil {
		return err
	}

	resp, err := c.Do(req)
	if err != nil {
		return fmt.Errorf("execute cloud account credential update request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusNoContent {
		return unexpectedStatus(resp, "updating cloud account credential")
	}

	return nil
}

func (c *Client) writeCloudAccount(ctx context.Context, method, networkID, name string, body AWSCloudAccountRequest) error {
	if c == nil {
		return fmt.Errorf("client is nil")
	}
	networkID = strings.TrimSpace(networkID)
	if networkID == "" {
		return fmt.Errorf("networkID must be provided")
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal cloud account request: %w", err)
	}

	path := fmt.Sprintf("/api/networks/%s/cloudAccounts", url.PathEscape(networkID))
	if method == http.MethodPatch {
		name = strings.TrimSpace(name)
		if name == "" {
			return fmt.Errorf("cloud account name must be provided")
		}
		path = path + "/" + url.PathEscape(name)
	}

	req, err := c.NewRequest(ctx, method, path, bytes.NewReader(payload))
	if err != nil {
		return err
	}

	resp, err := c.Do(req)
	if err != nil {
		return fmt.Errorf("execute cloud account %s request: %w", strings.ToLower(method), err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusNoContent {
		return unexpectedStatus(resp, "writing cloud account")
	}

	return nil
}

// DeleteCloudAccount removes a Forward cloud account setup by name.
func (c *Client) DeleteCloudAccount(ctx context.Context, networkID, name string) error {
	if c == nil {
		return fmt.Errorf("client is nil")
	}
	networkID = strings.TrimSpace(networkID)
	name = strings.TrimSpace(name)
	if networkID == "" || name == "" {
		return fmt.Errorf("networkID and cloud account name must be provided")
	}

	path := fmt.Sprintf("/api/networks/%s/cloudAccounts/%s", url.PathEscape(networkID), url.PathEscape(name))
	req, err := c.NewRequest(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return err
	}

	resp, err := c.Do(req)
	if err != nil {
		return fmt.Errorf("execute cloud account delete request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return unexpectedStatus(resp, "deleting cloud account")
	}

	return nil
}

func unexpectedStatus(resp *http.Response, operation string) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<14))
	return fmt.Errorf("unexpected status %d %s: %s", resp.StatusCode, operation, strings.TrimSpace(string(body)))
}
