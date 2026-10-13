// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// A prediction of nothing is refused at plan time: no change set is opened and
// nothing is left behind in Forward. The provider points at a closed port, so
// any call to Forward would fail the test with a different error.
func TestPredictedSnapshotRefusesNoChangeAtPlan(t *testing.T) {
	t.Parallel()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){
			"forward": providerserver.NewProtocol6WithError(New("test")()),
		},
		Steps: []resource.TestStep{
			{
				Config: `
provider "forward" {
  base_url = "http://127.0.0.1:1"
  username = "user"
  password = "pass"
}

resource "forward_predicted_snapshot" "test" {
  network_id = "net-1"
}
`,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`Set device_commands or firewall_rules`),
			},
		},
	})
}
