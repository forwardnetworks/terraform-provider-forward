// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package sdk

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func testClient(t *testing.T, baseURL string) *Client {
	t.Helper()
	client, err := NewClient(context.Background(), Config{BaseURL: baseURL, Username: "user", Password: "pass"})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return client
}

// The predict flow is four calls that must agree on one change set. A path or
// verb that is subtly wrong compiles and then fails as a 404 mid-apply, so these
// assert the wire contract rather than the Go types.
func TestPredictFlow(t *testing.T) {
	t.Parallel()

	var staged CloudChanges
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/networks/net-1/change-sets":
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["snapshotId"] != "snap-base" {
				t.Errorf("base snapshot = %q", body["snapshotId"])
			}
			_ = json.NewEncoder(w).Encode(ChangeSet{ID: "CHG-1", Name: body["name"]})
		case r.Method == http.MethodPost &&
			r.URL.Path == "/api/networks/net-1/change-sets/CHG-1/draft/devices/aws-src/cloud-changes":
			_ = json.NewDecoder(r.Body).Decode(&staged)
			_, _ = w.Write([]byte(`{}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/networks/net-1/change-sets/CHG-1/commits":
			if r.URL.Query().Get("note") != "demo" {
				t.Errorf("note = %q", r.URL.Query().Get("note"))
			}
			_, _ = w.Write([]byte(`{}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/networks/net-1/change-sets/CHG-1":
			if r.URL.Query().Get("action") != "predict" {
				t.Errorf("action = %q", r.URL.Query().Get("action"))
			}
			_ = json.NewEncoder(w).Encode(Snapshot{ID: "snap-9", State: "PROCESSING"})
		case r.Method == http.MethodGet && r.URL.Path == "/api/networks/net-1/snapshots/snap-9":
			_ = json.NewEncoder(w).Encode(SnapshotDetails{Snapshot: Snapshot{ID: "snap-9", State: "PROCESSED"}})
		case r.Method == http.MethodDelete && r.URL.Path == "/api/networks/net-1/change-sets/CHG-1":
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.RequestURI())
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := testClient(t, server.URL)
	ctx := context.Background()

	changeSet, err := client.CreateChangeSet(ctx, "net-1", "demo", "snap-base")
	if err != nil || changeSet.ID != "CHG-1" {
		t.Fatalf("CreateChangeSet() = %#v, %v", changeSet, err)
	}
	priority := int64(300)
	err = client.StageCloudChanges(ctx, "net-1", "CHG-1", "aws-src", CloudChanges{
		RouteChanges: []CloudRouteChange{{
			RouteTableID: "rtb-1", DestinationCidrBlock: "10.140.0.0/16",
			Target: &CloudRouteTarget{Kind: "TRANSIT_GATEWAY", ID: "tgw-1"},
		}},
		SecurityRuleChanges: []CloudSecurityRuleChange{{
			SecurityGroupID: "nsg-1",
			Addition: &CloudRuleAddition{
				Protocol: "TCP", TargetKind: "cidr", TargetID: "10.140.0.0/16",
				Name: "deny_demo", Priority: &priority, Deny: true,
			},
		}},
	})
	if err != nil {
		t.Fatalf("StageCloudChanges() = %v", err)
	}
	if staged.RouteChanges[0].Target.ID != "tgw-1" {
		t.Fatalf("route target did not survive: %#v", staged.RouteChanges[0])
	}
	// Azure orders its rulebase and lets a rule deny; both have to reach the wire.
	if staged.SecurityRuleChanges[0].Addition.Priority == nil || !staged.SecurityRuleChanges[0].Addition.Deny {
		t.Fatalf("priority and deny did not survive: %#v", staged.SecurityRuleChanges[0].Addition)
	}
	if err := client.CommitChangeSet(ctx, "net-1", "CHG-1", "demo"); err != nil {
		t.Fatalf("CommitChangeSet() = %v", err)
	}
	predicted, err := client.RunPredict(ctx, "net-1", "CHG-1", "demo")
	if err != nil || predicted.ID != "snap-9" {
		t.Fatalf("RunPredict() = %#v, %v", predicted, err)
	}
	details, err := client.AwaitSnapshot(ctx, "net-1", "snap-9", time.Millisecond)
	if err != nil || details.State != "PROCESSED" {
		t.Fatalf("AwaitSnapshot() = %#v, %v", details, err)
	}
	if err := client.DeleteChangeSet(ctx, "net-1", "CHG-1"); err != nil {
		t.Fatalf("DeleteChangeSet() = %v", err)
	}
}

// The newest snapshot is often a predicted one, and predicting from a prediction
// is refused -- so the base has to be chosen by trigger, not by recency.
func TestLatestCollectedSnapshotSkipsPredictions(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string][]Snapshot{"snapshots": {
			{ID: "snap-3", State: "PROCESSED", ProcessingTrigger: "PREDICT"},
			{ID: "snap-2", State: "PROCESSING", ProcessingTrigger: "COLLECTION"},
			{ID: "snap-1", State: "PROCESSED", ProcessingTrigger: "COLLECTION"},
		}})
	}))
	defer server.Close()

	got, err := testClient(t, server.URL).LatestCollectedSnapshot(context.Background(), "net-1")

	if err != nil {
		t.Fatalf("LatestCollectedSnapshot() = %v", err)
	}
	// snap-3 is newer but predicted; snap-2 is a collection but not finished.
	if got.ID != "snap-1" {
		t.Fatalf("chose %s, want snap-1", got.ID)
	}
}

// A plan that is not JSON is caught here rather than as a server-side 400,
// because the message a user sees should name the command that produces it.
func TestStageTerraformPlanRejectsNonJSON(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("request should not have been made: %s %s", r.Method, r.URL.Path)
	}))
	defer server.Close()

	err := testClient(t, server.URL).StageTerraformPlan(
		context.Background(), "net-1", "CHG-1", "aws-src", []byte("Terraform will perform the following actions"))

	if err == nil {
		t.Fatal("expected a plan that is not JSON to be refused")
	}
}
