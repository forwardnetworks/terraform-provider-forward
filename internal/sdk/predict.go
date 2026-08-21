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
	"time"
)

// ChangeSet is a set of staged, uncommitted changes against a base snapshot.
type ChangeSet struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	NetworkID string `json:"networkId"`
}

// CloudChanges is a provider-neutral statement of what a cloud collection source
// should look like after a change.
//
// A caller names a route table and a destination, or a security group and a rule,
// without knowing which provider's state files Predict will edit. A kind the
// provider has no predictor for is refused rather than ignored.
type CloudChanges struct {
	RouteChanges          []CloudRouteChange          `json:"routeChanges,omitempty"`
	SecurityRuleChanges   []CloudSecurityRuleChange   `json:"securityRuleChanges,omitempty"`
	TgwAssociationChanges []CloudTgwAssociationChange `json:"tgwAssociationChanges,omitempty"`
	TgwPropagationChanges []CloudTgwPropagationChange `json:"tgwPropagationChanges,omitempty"`
	VpnRouteChanges       []CloudVpnRouteChange       `json:"vpnRouteChanges,omitempty"`
}

// CloudRouteChange adds, retargets or removes one route. A destination names at
// most one route in a table, so restating it retargets; an absent Target removes.
type CloudRouteChange struct {
	RouteTableID         string            `json:"routeTableId"`
	DestinationCidrBlock string            `json:"destinationCidrBlock"`
	Target               *CloudRouteTarget `json:"target,omitempty"`
}

// CloudRouteTarget is where a route points. Kind is an AwsRouteTarget name for
// AWS and an AzureNextHopType name for Azure.
type CloudRouteTarget struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

// CloudSecurityRuleChange states exactly one of Addition or RemovedRuleID.
type CloudSecurityRuleChange struct {
	SecurityGroupID string             `json:"securityGroupId"`
	Addition        *CloudRuleAddition `json:"addition,omitempty"`
	RemovedRuleID   string             `json:"removedRuleId,omitempty"`
}

// CloudRuleAddition is a security rule stated as its own fields. Name, Priority
// and Deny are what Azure needs on top of what AWS does: it names each rule,
// orders them by priority, and lets one deny.
type CloudRuleAddition struct {
	Egress      bool   `json:"egress"`
	Protocol    string `json:"protocol"`
	FromPort    *int64 `json:"fromPort,omitempty"`
	ToPort      *int64 `json:"toPort,omitempty"`
	TargetKind  string `json:"targetKind"`
	TargetID    string `json:"targetId"`
	Description string `json:"description"`
	Name        string `json:"name,omitempty"`
	Priority    *int64 `json:"priority,omitempty"`
	Deny        bool   `json:"deny,omitempty"`
}

// CloudTgwAssociationChange retargets an attachment; an absent RouteTableID
// disassociates it.
type CloudTgwAssociationChange struct {
	AttachmentID string `json:"attachmentId"`
	RouteTableID string `json:"routeTableId,omitempty"`
}

// CloudTgwPropagationChange states whether an attachment's prefixes are learned
// into a route table. The pair is the identity, so Enabled says whether the
// membership exists.
type CloudTgwPropagationChange struct {
	RouteTableID string `json:"routeTableId"`
	AttachmentID string `json:"attachmentId"`
	Enabled      bool   `json:"enabled"`
}

// CloudVpnRouteChange states a destination a VPN connection carries. Only a
// statically routed connection states its own; a BGP connection learns them from
// the peer and is refused.
type CloudVpnRouteChange struct {
	VpnConnectionID      string `json:"vpnConnectionId"`
	DestinationCidrBlock string `json:"destinationCidrBlock"`
	Present              bool   `json:"present"`
}

// CreateChangeSet opens a change set against a base snapshot.
//
// The base must be a collected snapshot: predicting from a prediction is refused
// by the server, and the newest snapshot in a network is often a predicted one.
func (c *Client) CreateChangeSet(ctx context.Context, networkID, name, baseSnapshotID string) (*ChangeSet, error) {
	if c == nil {
		return nil, fmt.Errorf("client is nil")
	}
	networkID = strings.TrimSpace(networkID)
	if networkID == "" {
		return nil, fmt.Errorf("networkID must be provided")
	}
	body := map[string]string{"name": name}
	if s := strings.TrimSpace(baseSnapshotID); s != "" {
		body["snapshotId"] = s
	}
	path := fmt.Sprintf("/api/networks/%s/change-sets", url.PathEscape(networkID))
	changeSet := new(ChangeSet)
	if err := c.doJSON(ctx, http.MethodPost, path, body, changeSet); err != nil {
		return nil, err
	}
	return changeSet, nil
}

// DeleteChangeSet discards a change set and the drafts staged on it.
func (c *Client) DeleteChangeSet(ctx context.Context, networkID, changeSetID string) error {
	if c == nil {
		return fmt.Errorf("client is nil")
	}
	path := fmt.Sprintf("/api/networks/%s/change-sets/%s",
		url.PathEscape(strings.TrimSpace(networkID)), url.PathEscape(strings.TrimSpace(changeSetID)))
	return c.doJSON(ctx, http.MethodDelete, path, nil, nil)
}

// StageCloudChanges replaces the cloud changes staged for a collection source.
//
// sourceName names a collection *source*, not a device: a cloud source models
// many devices, none of which is a device by the source's own name.
func (c *Client) StageCloudChanges(
	ctx context.Context,
	networkID, changeSetID, sourceName string,
	changes CloudChanges,
) error {
	return c.stageCloud(ctx, networkID, changeSetID, sourceName, "", changes)
}

// StageTerraformPlan translates `terraform show -json` output into cloud changes
// and stages them. The server reports what it could not translate rather than
// dropping it silently.
func (c *Client) StageTerraformPlan(
	ctx context.Context,
	networkID, changeSetID, sourceName string,
	planJSON []byte,
) error {
	if !json.Valid(planJSON) {
		return fmt.Errorf("terraform plan is not valid JSON; pass the output of `terraform show -json`")
	}
	return c.stageCloud(ctx, networkID, changeSetID, sourceName, "fromTerraformPlan", json.RawMessage(planJSON))
}

func (c *Client) stageCloud(
	ctx context.Context,
	networkID, changeSetID, sourceName, action string,
	body any,
) error {
	if c == nil {
		return fmt.Errorf("client is nil")
	}
	for name, value := range map[string]string{
		"networkID": networkID, "changeSetID": changeSetID, "sourceName": sourceName,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s must be provided", name)
		}
	}
	path := fmt.Sprintf("/api/networks/%s/change-sets/%s/draft/devices/%s/cloud-changes",
		url.PathEscape(strings.TrimSpace(networkID)),
		url.PathEscape(strings.TrimSpace(changeSetID)),
		url.PathEscape(strings.TrimSpace(sourceName)))
	if action != "" {
		path += "?" + url.Values{"action": []string{action}}.Encode()
	}
	return c.doJSON(ctx, http.MethodPost, path, body, nil)
}

// CommitChangeSet records the staged draft as a commit, which is what Predict
// then runs against.
func (c *Client) CommitChangeSet(ctx context.Context, networkID, changeSetID, note string) error {
	if c == nil {
		return fmt.Errorf("client is nil")
	}
	path := fmt.Sprintf("/api/networks/%s/change-sets/%s/commits?%s",
		url.PathEscape(strings.TrimSpace(networkID)),
		url.PathEscape(strings.TrimSpace(changeSetID)),
		url.Values{"note": []string{note}}.Encode())
	return c.doJSON(ctx, http.MethodPost, path, nil, nil)
}

// RunPredict starts predictive analysis and returns the predicted snapshot,
// which is still processing when this returns.
func (c *Client) RunPredict(ctx context.Context, networkID, changeSetID, note string) (*Snapshot, error) {
	if c == nil {
		return nil, fmt.Errorf("client is nil")
	}
	path := fmt.Sprintf("/api/networks/%s/change-sets/%s?%s",
		url.PathEscape(strings.TrimSpace(networkID)),
		url.PathEscape(strings.TrimSpace(changeSetID)),
		url.Values{"action": []string{"predict"}, "note": []string{note}}.Encode())
	snapshot := new(Snapshot)
	if err := c.doJSON(ctx, http.MethodPost, path, nil, snapshot); err != nil {
		return nil, err
	}
	return snapshot, nil
}

// AwaitSnapshot polls until the snapshot reaches a terminal state.
//
// A predicted snapshot that is still processing has no model to read, so a
// caller that returns before this would hand back an id nothing can query yet.
func (c *Client) AwaitSnapshot(ctx context.Context, networkID, snapshotID string, interval time.Duration) (*SnapshotDetails, error) {
	if interval <= 0 {
		interval = 10 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		details, err := c.GetSnapshot(ctx, networkID, snapshotID)
		if err == nil {
			switch strings.ToUpper(details.State) {
			case "PROCESSED":
				return details, nil
			case "FAILED", "CANCELED", "TIMED_OUT", "RESTORE_FAILED":
				return details, fmt.Errorf("snapshot %s ended in state %s", snapshotID, details.State)
			}
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}

// LatestCollectedSnapshot returns the newest snapshot that came from a
// collection rather than from a prediction.
//
// Predicting from a prediction is refused, and after any predict the newest
// snapshot in the network is a predicted one -- so "the latest snapshot" is the
// wrong base almost as often as it is the right one.
func (c *Client) LatestCollectedSnapshot(ctx context.Context, networkID string) (*Snapshot, error) {
	limit := 40
	snapshots, err := c.ListSnapshots(ctx, networkID, SnapshotListOptions{Limit: &limit})
	if err != nil {
		return nil, err
	}
	for i := range snapshots {
		if !strings.EqualFold(snapshots[i].ProcessingTrigger, "PREDICT") &&
			strings.EqualFold(snapshots[i].State, "PROCESSED") {
			return &snapshots[i], nil
		}
	}
	return nil, fmt.Errorf("no collected snapshot found in the most recent %d for network %s", len(snapshots), networkID)
}

// doJSON performs a request whose body and response are JSON, decoding into out
// when out is non-nil.
func (c *Client) doJSON(ctx context.Context, method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode request body: %w", err)
		}
		reader = strings.NewReader(string(encoded))
	}
	req, err := c.NewRequest(ctx, method, path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.Do(req)
	if err != nil {
		return fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		payload, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<14))
		return fmt.Errorf("unexpected status %d for %s %s: %s", resp.StatusCode, method, path, string(payload))
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}
