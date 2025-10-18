// Copyright (c) HashiCorp, Inc.

package provider

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

const (
	testNetworkID  = "network-123"
	testSnapshotID = "snapshot-1"
)

func TestAccDataSourceVersionAndSnapshots(t *testing.T) {
	versionPayload := `{"build":"ee9b380","release":"21.50.1-03","version":"21.50.1"}`
	snapshotsPayload := fmt.Sprintf(`{
        "id": "%s",
        "name": "Primary",
        "orgId": "42",
        "snapshots": [
            {
                "id": "%s",
                "state": "PROCESSED",
                "processingTrigger": "COLLECTION",
                "creationDateMillis": 1700000000000,
                "processedAtMillis": 1700000010000
            },
            {
                "id": "snap-2",
                "state": "FAILED",
                "processingTrigger": "COLLECTION",
                "isDraft": true
            }
        ]
    }`, testNetworkID, testSnapshotID)
	checksPayload := `[
        {
            "id": "check-1",
            "name": "Critical Reachability",
            "status": "PASS",
            "priority": "HIGH",
            "numViolations": 0,
            "enabled": true,
            "creationDateMillis": 1700000000000,
            "executionDateMillis": 1700000015000
        },
        {
            "id": "check-2",
            "name": "Intent Regression",
            "status": "FAIL",
            "priority": "MEDIUM",
            "numViolations": 1,
            "enabled": true,
            "tags": ["pre-change"],
            "executionDurationMillis": 1500
        }
    ]`
	nqePayload := fmt.Sprintf(`{
        "snapshotId": "%s",
        "items": [
            {"fields": {"device": "leaf1", "status": "permit"}}
        ],
        "totalNumItems": 1
    }`, testSnapshotID)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/version":
			fmt.Fprint(w, versionPayload)
		case fmt.Sprintf("/api/networks/%s/snapshots", testNetworkID):
			fmt.Fprint(w, snapshotsPayload)
		case fmt.Sprintf("/api/snapshots/%s/checks", testSnapshotID):
			fmt.Fprint(w, checksPayload)
		case "/api/nqe":
			if r.Method != http.MethodPost {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, nqePayload)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	t.Setenv("FORWARD_API_KEY", "test-token")

	config := fmt.Sprintf(`
variable "forward_api_key" {
  type        = string
  default     = "%[2]s"
  sensitive   = true
}

variable "forward_base_url" {
  type    = string
  default = "%[1]s"
}

variable "forward_network_id" {
  type    = string
  default = "%[3]s"
}

variable "forward_insecure" {
  type    = bool
  default = false
}

provider "forward" {
  base_url   = var.forward_base_url
  network_id = var.forward_network_id
  api_key    = var.forward_api_key
  insecure   = var.forward_insecure
}

data "forward_version" "current" {}

data "forward_snapshots" "all" {
  limit = 2
}

data "forward_intent_checks" "snapshot_checks" {
  snapshot_id = data.forward_snapshots.all.snapshots[0].id
}

data "forward_nqe_query" "latest_acl" {
  snapshot_id = data.forward_snapshots.all.snapshots[0].id
  query       = "SELECT device FROM devices"
  parameters = {
    severity = "\"critical\""
  }
  limit = 5
}
`, server.URL, "test-token", testNetworkID)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"data.forward_version.current",
						tfjsonpath.New("version"),
						knownvalue.StringExact("21.50.1"),
					),
					statecheck.ExpectKnownValue(
						"data.forward_snapshots.all",
						tfjsonpath.New("snapshots[0].id"),
						knownvalue.StringExact("snap-1"),
					),
					statecheck.ExpectKnownValue(
						"data.forward_snapshots.all",
						tfjsonpath.New("snapshots[0].processed_at_millis"),
						knownvalue.Int64Exact(1700000010000),
					),
					statecheck.ExpectKnownValue(
						"data.forward_snapshots.all",
						tfjsonpath.New("snapshots[1].is_draft"),
						knownvalue.Bool(true),
					),
					statecheck.ExpectKnownValue(
						"data.forward_intent_checks.snapshot_checks",
						tfjsonpath.New("pass_count"),
						knownvalue.Int64Exact(1),
					),
					statecheck.ExpectKnownValue(
						"data.forward_intent_checks.snapshot_checks",
						tfjsonpath.New("checks[1].status"),
						knownvalue.StringExact("FAIL"),
					),
					statecheck.ExpectKnownValue(
						"data.forward_nqe_query.latest_acl",
						tfjsonpath.New("items_json[0]"),
						knownvalue.StringExact(`{"fields":{"device":"leaf1","status":"permit"}}`),
					),
					statecheck.ExpectKnownValue(
						"data.forward_nqe_query.latest_acl",
						tfjsonpath.New("total_items"),
						knownvalue.Int64Exact(1),
					),
				},
			},
		},
	})
}
