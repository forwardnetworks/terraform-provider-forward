// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// Copyright (c) HashiCorp, Inc.

package provider

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// The fake serves the routes a live appserver serves and no others: collection
// runs through the collector-task queue, the snapshot is found by the task id
// it records, and there is no per-snapshot metadata route -- the shape that
// broke both collecting and waiting.
func TestSnapshotResourceCreate(t *testing.T) {
	t.Parallel()

	var polls int
	taskStatus := "RUNNING"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/collector-tasks":
			_, _ = w.Write([]byte(`{"taskId":"P1021"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/collector-tasks/P1021":
			state := taskStatus
			taskStatus = "SUCCEEDED"
			_, _ = w.Write([]byte(`{"id":"P1021","type":"NETWORK_COLLECTION","status":"` + state + `"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/networks/net-1/snapshots":
			polls++
			state := "IN_PROGRESS"
			if polls > 2 {
				state = "PROCESSED"
			}
			_, _ = w.Write([]byte(`{"snapshots":[{"id":"snap-1","state":"` + state + `",
			  "processingTrigger":"COLLECTION","collectionTaskId":"1021",
			  "createdAt":"2023-11-14T22:13:20.000Z","processedAt":"2023-11-14T22:13:30.000Z"}]}`))
		case r.Method == http.MethodPatch && r.URL.Path == "/api/snapshots/snap-1":
			_, _ = w.Write([]byte(`{"id":"snap-1","note":"test","state":"IN_PROGRESS",
			  "processingTrigger":"COLLECTION","collectionTaskId":"1021",
			  "createdAt":"2023-11-14T22:13:20.000Z"}`))
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			// A 404 here stands in for the metadata route this build lacks.
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	providerFactory := providerserver.NewProtocol6WithError(New("test")())

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){
			"forward": providerFactory,
		},
		Steps: []resource.TestStep{
			{
				Config: snapshotTestConfig(server.URL),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("forward_snapshot.test", "id", "snap-1"),
					// Waiting has to actually observe the transition; the old
					// test asserted nothing and passed while waiting was broken.
					resource.TestCheckResourceAttr("forward_snapshot.test", "state", "PROCESSED"),
					resource.TestCheckResourceAttr("forward_snapshot.test", "note", "test"),
					resource.TestCheckResourceAttr("forward_snapshot.test", "created_at", "2023-11-14T22:13:20.000Z"),
					resource.TestCheckResourceAttr("forward_snapshot.test", "processed_at", "2023-11-14T22:13:30.000Z"),
				),
			},
		},
	})
}

func snapshotTestConfig(host string) string {
	return fmt.Sprintf(`
provider "forward" {
  base_url   = "%s"
  network_id = "net-1"
  username = "user"
  password = "pass"
}

resource "forward_snapshot" "test" {
  network_id            = "net-1"
  note                  = "test"
  wait_for_processed    = true
  poll_interval_seconds = 1
  timeout_seconds       = 30
}
`, host)
}
