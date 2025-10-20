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

func TestSnapshotResourceCreate(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			_, _ = w.Write([]byte(`{"id":"snap-1","state":"PROCESSING"}`))
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"id":"snap-1","state":"PROCESSED"}`))
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected method: %s", r.Method)
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
			},
		},
	})
}

func snapshotTestConfig(host string) string {
	return fmt.Sprintf(`
provider "forward" {
  base_url   = "%s"
  network_id = "net-1"
  api_key    = "token"
}

resource "forward_snapshot" "test" {
  network_id         = "net-1"
  note               = "test"
  wait_for_processed = false
}
`, host)
}
