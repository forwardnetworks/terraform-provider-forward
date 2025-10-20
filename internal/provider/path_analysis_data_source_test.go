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

func TestPathAnalysisDataSource(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/networks/net-1/paths" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"srcIpLocationType":"INTERFACE","dstIpLocationType":"INTERFACE","info":{"paths":[{"forwardingOutcome":"DELIVERED","securityOutcome":"PERMITTED","hops":[]}]} }`))
	}))
	defer server.Close()

	providerFactory := providerserver.NewProtocol6WithError(New("test")())

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){
			"forward": providerFactory,
		},
		Steps: []resource.TestStep{
			{
				Config: pathAnalysisTestConfig(server.URL),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.forward_path_analysis.test", "paths.#", "1"),
				),
			},
		},
	})
}

func pathAnalysisTestConfig(host string) string {
	return fmt.Sprintf(`
provider "forward" {
  base_url   = "%s"
  network_id = "net-1"
  api_key    = "token"
}

data "forward_path_analysis" "test" {
  network_id = "net-1"
  src_ip     = "10.0.0.2"
  dst_ip     = "10.0.0.1"
}
`, host)
}
