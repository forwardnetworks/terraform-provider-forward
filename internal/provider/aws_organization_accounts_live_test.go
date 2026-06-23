// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"os"
	"testing"
)

func TestAccAWSOrganizationAccountsDiscoveryLive(t *testing.T) {
	if os.Getenv("TF_ACC_AWS_ORG") != "1" {
		t.Skip("set TF_ACC_AWS_ORG=1 to run live AWS Organizations discovery")
	}

	roleName := os.Getenv("TF_ACC_AWS_ORG_ROLE_NAME")
	if roleName == "" {
		roleName = "ForwardNetworksReadOnly"
	}

	result, err := discoverAWSOrganizationAccounts(context.Background(), awsOrganizationDiscoveryOptions{
		Region:     "us-east-1",
		RoleName:   roleName,
		ExternalID: "terraform-provider-forward-live-test",
	})
	if err != nil {
		t.Fatalf("discover AWS Organizations accounts: %v", err)
	}
	if result.OrganizationID == "" {
		t.Fatalf("expected organization ID")
	}
	if len(result.Accounts) == 0 {
		t.Fatalf("expected at least one AWS account")
	}
	for _, account := range result.Accounts {
		if account.ID == "" || account.RoleArn == "" {
			t.Fatalf("discovered incomplete account: %#v", account)
		}
	}
}
