// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"

	"github.com/forwardnetworks/terraform-provider-forward/internal/sdk"
)

func TestAccAWSCloudAccountResourceCreatesAndUpdates(t *testing.T) {
	var stored *sdk.AWSCloudAccountRequest
	var postCount int
	var patchCount int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == fmt.Sprintf("/api/networks/%s/cloudAccounts/aws/assumeRole/externalId", testNetworkID):
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"externalId":"Org:55"}`)
		case r.Method == http.MethodGet && r.URL.Path == fmt.Sprintf("/api/networks/%s/cloudAccounts", testNetworkID):
			w.Header().Set("Content-Type", "application/json")
			if stored == nil {
				fmt.Fprint(w, `[]`)
				return
			}
			_ = json.NewEncoder(w).Encode([]any{cloudAccountResponse(*stored)})
		case r.Method == http.MethodPost && r.URL.Path == fmt.Sprintf("/api/networks/%s/cloudAccounts", testNetworkID):
			postCount++
			var payload sdk.AWSCloudAccountRequest
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			stored = &payload
			w.WriteHeader(http.StatusCreated)
		case r.Method == http.MethodPatch && r.URL.Path == fmt.Sprintf("/api/networks/%s/cloudAccounts/%s", testNetworkID, "org-aws"):
			patchCount++
			var raw map[string]any
			if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if _, ok := raw["useForwardAccountToAssumeRole"]; ok {
				http.Error(w, "PATCH must not include useForwardAccountToAssumeRole", http.StatusBadRequest)
				return
			}
			encoded, err := json.Marshal(raw)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			var payload sdk.AWSCloudAccountRequest
			if err := json.Unmarshal(encoded, &payload); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			stored = &payload
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	t.Setenv("FORWARD_USERNAME", "user")
	t.Setenv("FORWARD_PASSWORD", "pass")

	configOne := awsCloudAccountConfig(server.URL, `
  assume_role_infos = [{
    account_id   = "111111111111"
    account_name = "prod"
    role_arn     = "arn:aws:iam::111111111111:role/ForwardNetworksReadOnly"
    external_id  = data.forward_aws_assume_role_external_id.current.external_id
    enabled      = true
  }]
`)

	configTwo := awsCloudAccountConfig(server.URL, `
  assume_role_infos = [
    {
      account_id   = "111111111111"
      account_name = "prod"
      role_arn     = "arn:aws:iam::111111111111:role/ForwardNetworksReadOnly"
      external_id  = data.forward_aws_assume_role_external_id.current.external_id
      enabled      = true
    },
    {
      account_id   = "222222222222"
      account_name = "dev"
      role_arn     = "arn:aws:iam::222222222222:role/ForwardNetworksReadOnly"
      external_id  = data.forward_aws_assume_role_external_id.current.external_id
      enabled      = true
    }
  ]
`)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: configOne,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"data.forward_aws_assume_role_external_id.current",
						tfjsonpath.New("external_id"),
						knownvalue.StringExact("Org:55"),
					),
					statecheck.ExpectKnownValue(
						"forward_aws_cloud_account.org",
						tfjsonpath.New("account_count"),
						knownvalue.Int64Exact(1),
					),
				},
			},
			{
				Config: configTwo,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"forward_aws_cloud_account.org",
						tfjsonpath.New("account_count"),
						knownvalue.Int64Exact(2),
					),
				},
			},
		},
	})

	if postCount != 1 {
		t.Fatalf("expected one POST, got %d", postCount)
	}
	if patchCount != 1 {
		t.Fatalf("expected one PATCH, got %d", patchCount)
	}
	if stored == nil || len(stored.AssumeRoleInfos) != 2 {
		t.Fatalf("expected stored payload with two assume role infos, got %#v", stored)
	}
}

func TestAccAWSCloudAccountResourceAdoptsExistingSetup(t *testing.T) {
	stored := awsCloudAccountRequestWithAccounts(
		sdk.AWSAssumeRoleInfo{
			AccountID:   "111111111111",
			AccountName: "prod",
			RoleArn:     "arn:aws:iam::111111111111:role/ForwardNetworksReadOnly",
			ExternalID:  "Org:55",
			Enabled:     true,
		},
	)
	var postCount int
	var patchCount int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == fmt.Sprintf("/api/networks/%s/cloudAccounts/aws/assumeRole/externalId", testNetworkID):
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"externalId":"Org:55"}`)
		case r.Method == http.MethodGet && r.URL.Path == fmt.Sprintf("/api/networks/%s/cloudAccounts", testNetworkID):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]any{cloudAccountResponse(*stored)})
		case r.Method == http.MethodPost && r.URL.Path == fmt.Sprintf("/api/networks/%s/cloudAccounts", testNetworkID):
			postCount++
			http.Error(w, "create should not be called for an existing setup", http.StatusConflict)
		case r.Method == http.MethodPatch && r.URL.Path == fmt.Sprintf("/api/networks/%s/cloudAccounts/%s", testNetworkID, "org-aws"):
			patchCount++
			var payload sdk.AWSCloudAccountRequest
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			payload.UseForwardAccountToAssumeRole = stored.UseForwardAccountToAssumeRole
			stored = &payload
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	t.Setenv("FORWARD_USERNAME", "user")
	t.Setenv("FORWARD_PASSWORD", "pass")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: awsCloudAccountConfig(server.URL, `
  assume_role_infos = [
    {
      account_id   = "111111111111"
      account_name = "prod"
      role_arn     = "arn:aws:iam::111111111111:role/ForwardNetworksReadOnly"
      external_id  = data.forward_aws_assume_role_external_id.current.external_id
      enabled      = true
    },
    {
      account_id   = "222222222222"
      account_name = "dev"
      role_arn     = "arn:aws:iam::222222222222:role/ForwardNetworksReadOnly"
      external_id  = data.forward_aws_assume_role_external_id.current.external_id
      enabled      = true
    }
  ]
`),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"forward_aws_cloud_account.org",
						tfjsonpath.New("account_count"),
						knownvalue.Int64Exact(2),
					),
				},
			},
		},
	})

	if postCount != 0 {
		t.Fatalf("expected no POST calls for existing setup, got %d", postCount)
	}
	if patchCount != 1 {
		t.Fatalf("expected one PATCH for existing setup, got %d", patchCount)
	}
}

func TestAccAWSCloudAccountResourceBlocksAccountRemovalsByDefault(t *testing.T) {
	stored := awsCloudAccountRequestWithAccounts(
		sdk.AWSAssumeRoleInfo{
			AccountID:   "111111111111",
			AccountName: "prod",
			RoleArn:     "arn:aws:iam::111111111111:role/ForwardNetworksReadOnly",
			ExternalID:  "Org:55",
			Enabled:     true,
		},
		sdk.AWSAssumeRoleInfo{
			AccountID:   "222222222222",
			AccountName: "dev",
			RoleArn:     "arn:aws:iam::222222222222:role/ForwardNetworksReadOnly",
			ExternalID:  "Org:55",
			Enabled:     true,
		},
	)
	var patchCount int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == fmt.Sprintf("/api/networks/%s/cloudAccounts/aws/assumeRole/externalId", testNetworkID):
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"externalId":"Org:55"}`)
		case r.Method == http.MethodGet && r.URL.Path == fmt.Sprintf("/api/networks/%s/cloudAccounts", testNetworkID):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]any{cloudAccountResponse(*stored)})
		case r.Method == http.MethodPatch && r.URL.Path == fmt.Sprintf("/api/networks/%s/cloudAccounts/%s", testNetworkID, "org-aws"):
			patchCount++
			http.Error(w, "patch should not be called when removals are not confirmed", http.StatusBadRequest)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	t.Setenv("FORWARD_USERNAME", "user")
	t.Setenv("FORWARD_PASSWORD", "pass")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      awsCloudAccountConfig(server.URL, oneAccountAssumeRoleBlock()),
				ExpectError: regexp.MustCompile("AWS Account Removals Require Confirmation"),
			},
		},
	})

	if patchCount != 0 {
		t.Fatalf("expected no PATCH calls when removals are not confirmed, got %d", patchCount)
	}
}

func TestAccAWSCloudAccountResourceAllowsAccountRemovals(t *testing.T) {
	stored := awsCloudAccountRequestWithAccounts(
		sdk.AWSAssumeRoleInfo{
			AccountID:   "111111111111",
			AccountName: "prod",
			RoleArn:     "arn:aws:iam::111111111111:role/ForwardNetworksReadOnly",
			ExternalID:  "Org:55",
			Enabled:     true,
		},
		sdk.AWSAssumeRoleInfo{
			AccountID:   "222222222222",
			AccountName: "dev",
			RoleArn:     "arn:aws:iam::222222222222:role/ForwardNetworksReadOnly",
			ExternalID:  "Org:55",
			Enabled:     true,
		},
	)
	var patchCount int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == fmt.Sprintf("/api/networks/%s/cloudAccounts/aws/assumeRole/externalId", testNetworkID):
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"externalId":"Org:55"}`)
		case r.Method == http.MethodGet && r.URL.Path == fmt.Sprintf("/api/networks/%s/cloudAccounts", testNetworkID):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]any{cloudAccountResponse(*stored)})
		case r.Method == http.MethodPatch && r.URL.Path == fmt.Sprintf("/api/networks/%s/cloudAccounts/%s", testNetworkID, "org-aws"):
			patchCount++
			var payload sdk.AWSCloudAccountRequest
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			payload.UseForwardAccountToAssumeRole = stored.UseForwardAccountToAssumeRole
			stored = &payload
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	t.Setenv("FORWARD_USERNAME", "user")
	t.Setenv("FORWARD_PASSWORD", "pass")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: awsCloudAccountConfig(server.URL, `
  allow_account_removals = true

`+oneAccountAssumeRoleBlock()),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"forward_aws_cloud_account.org",
						tfjsonpath.New("account_count"),
						knownvalue.Int64Exact(1),
					),
				},
			},
		},
	})

	if patchCount != 1 {
		t.Fatalf("expected one PATCH when removals are confirmed, got %d", patchCount)
	}
	if len(stored.AssumeRoleInfos) != 1 || stored.AssumeRoleInfos[0].AccountID != "111111111111" {
		t.Fatalf("unexpected stored accounts after removal: %#v", stored.AssumeRoleInfos)
	}
}

func TestAccAWSCloudAccountResourceStaticKeysUpdateCredential(t *testing.T) {
	var stored *sdk.AWSCloudAccountRequest
	var credential sdk.AWSCloudAccountCredentialRequest
	var postCount int
	var patchCount int
	var credentialCount int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == fmt.Sprintf("/api/networks/%s/cloudAccounts", testNetworkID):
			w.Header().Set("Content-Type", "application/json")
			if stored == nil {
				fmt.Fprint(w, `[]`)
				return
			}
			_ = json.NewEncoder(w).Encode([]any{cloudAccountResponse(*stored)})
		case r.Method == http.MethodPost && r.URL.Path == fmt.Sprintf("/api/networks/%s/cloudAccounts", testNetworkID):
			postCount++
			var payload sdk.AWSCloudAccountRequest
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if payload.UseForwardAccountToAssumeRole == nil || *payload.UseForwardAccountToAssumeRole {
				http.Error(w, "static-key create must set useForwardAccountToAssumeRole=false", http.StatusBadRequest)
				return
			}
			if payload.Username == "" || payload.Password == "" {
				http.Error(w, "static-key create must include username/password", http.StatusBadRequest)
				return
			}
			stored = &payload
			w.WriteHeader(http.StatusCreated)
		case r.Method == http.MethodPatch && r.URL.Path == fmt.Sprintf("/api/networks/%s/cloudAccounts/%s", testNetworkID, "org-aws"):
			patchCount++
			var raw map[string]any
			if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if _, ok := raw["useForwardAccountToAssumeRole"]; ok {
				http.Error(w, "PATCH must not include useForwardAccountToAssumeRole", http.StatusBadRequest)
				return
			}
			if _, ok := raw["username"]; ok {
				http.Error(w, "PATCH must not include username", http.StatusBadRequest)
				return
			}
			if _, ok := raw["password"]; ok {
				http.Error(w, "PATCH must not include password", http.StatusBadRequest)
				return
			}
			encoded, err := json.Marshal(raw)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			var payload sdk.AWSCloudAccountRequest
			if err := json.Unmarshal(encoded, &payload); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			stored.AssumeRoleInfos = payload.AssumeRoleInfos
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && r.URL.Path == fmt.Sprintf("/api/networks/%s/cloudAccounts/%s/credential", testNetworkID, "org-aws"):
			credentialCount++
			if err := json.NewDecoder(r.Body).Decode(&credential); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	t.Setenv("FORWARD_USERNAME", "user")
	t.Setenv("FORWARD_PASSWORD", "pass")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: awsCloudAccountStaticKeysConfig(server.URL, "secret-one"),
			},
			{
				Config: awsCloudAccountStaticKeysConfig(server.URL, "secret-two"),
			},
		},
	})

	if postCount != 1 {
		t.Fatalf("expected one POST, got %d", postCount)
	}
	if patchCount != 1 {
		t.Fatalf("expected one PATCH, got %d", patchCount)
	}
	if credentialCount != 1 {
		t.Fatalf("expected one credential update, got %d", credentialCount)
	}
	if credential.Type != "AWS" || credential.Username != "AKIAEXAMPLE" || credential.Password != "secret-two" {
		t.Fatalf("unexpected credential payload: %#v", credential)
	}
}

func TestBuildAWSCloudAccountRequestInstanceProfile(t *testing.T) {
	regions, diags := types.SetValue(types.StringType, []attr.Value{types.StringValue("us-east-1")})
	if diags.HasError() {
		t.Fatalf("build regions: %v", diags)
	}
	request, diags := buildAWSCloudAccountRequest(t.Context(), awsCloudAccountResourceModel{
		Name:           types.StringValue("org-aws"),
		CredentialMode: types.StringValue(awsCredentialModeInstanceProfile),
		Regions:        regions,
		AssumeRoleInfos: []awsAssumeRoleInfoModel{{
			AccountID: types.StringValue("111111111111"),
			RoleArn:   types.StringValue("arn:aws:iam::111111111111:role/ForwardNetworksReadOnly"),
			Enabled:   types.BoolValue(true),
		}},
	})
	if diags.HasError() {
		t.Fatalf("build request returned diagnostics: %v", diags)
	}
	if request.UseForwardAccountToAssumeRole == nil || *request.UseForwardAccountToAssumeRole {
		t.Fatalf("instance-profile mode must set useForwardAccountToAssumeRole=false: %#v", request.UseForwardAccountToAssumeRole)
	}
	if request.Username != "" || request.Password != "" {
		t.Fatalf("instance-profile mode must not include username/password: %#v", request)
	}
}

func TestAccountRemovalDiagnostics(t *testing.T) {
	existing := &sdk.CloudAccount{
		Name: "org-aws",
		AssumeRoleInfos: []sdk.AWSAssumeRoleInfo{
			{
				AccountID:   "111111111111",
				AccountName: "prod",
				RoleArn:     "arn:aws:iam::111111111111:role/ForwardNetworksReadOnly",
			},
			{
				AccountName: "dev",
				RoleArn:     "arn:aws-us-gov:iam::222222222222:role/ForwardNetworksReadOnly",
			},
		},
	}
	planned := []sdk.AWSAssumeRoleInfo{
		{
			AccountID: "111111111111",
			RoleArn:   "arn:aws:iam::111111111111:role/ForwardNetworksReadOnly",
		},
	}

	diags := accountRemovalDiagnostics(awsCloudAccountResourceModel{}, existing, planned)
	if !diags.HasError() {
		t.Fatal("expected removals to be blocked by default")
	}
	if !strings.Contains(diags[0].Detail(), "222222222222 (dev)") {
		t.Fatalf("expected removed account detail, got %q", diags[0].Detail())
	}

	diags = accountRemovalDiagnostics(awsCloudAccountResourceModel{AllowAccountRemovals: types.BoolValue(true)}, existing, planned)
	if diags.HasError() {
		t.Fatalf("expected confirmed removals to pass, got %v", diags)
	}
}

func awsCloudAccountConfig(baseURL, assumeRoleBlock string) string {
	return fmt.Sprintf(`
provider "forward" {
  base_url   = %[1]q
  network_id = %[2]q
  username = "user"
  password = "pass"
}

data "forward_aws_assume_role_external_id" "current" {}

resource "forward_aws_cloud_account" "org" {
  name    = "org-aws"
  collect = true
  regions = ["us-east-1", "us-west-2"]

%[3]s
}
`, baseURL, testNetworkID, strings.TrimSpace(assumeRoleBlock))
}

func oneAccountAssumeRoleBlock() string {
	return `
  assume_role_infos = [{
    account_id   = "111111111111"
    account_name = "prod"
    role_arn     = "arn:aws:iam::111111111111:role/ForwardNetworksReadOnly"
    external_id  = data.forward_aws_assume_role_external_id.current.external_id
    enabled      = true
  }]
`
}

func awsCloudAccountStaticKeysConfig(baseURL, secret string) string {
	return fmt.Sprintf(`
provider "forward" {
  base_url   = %[1]q
  network_id = %[2]q
  username = "user"
  password = "pass"
}

resource "forward_aws_cloud_account" "org" {
  name    = "org-aws"
  collect = true
  regions = ["us-east-1"]

  credential_mode             = "static-keys"
  collector_access_key_id     = "AKIAEXAMPLE"
  collector_secret_access_key = %[3]q

  assume_role_infos = [{
    account_id = "111111111111"
    role_arn   = "arn:aws:iam::111111111111:role/ForwardNetworksReadOnly"
    enabled    = true
  }]
}
`, baseURL, testNetworkID, secret)
}

func awsCloudAccountRequestWithAccounts(accounts ...sdk.AWSAssumeRoleInfo) *sdk.AWSCloudAccountRequest {
	collect := true
	useForward := true
	return &sdk.AWSCloudAccountRequest{
		Type:                          "AWS",
		Name:                          "org-aws",
		Collect:                       &collect,
		Regions:                       map[string]int64{"us-east-1": 1, "us-west-2": 1},
		AssumeRoleInfos:               accounts,
		UseForwardAccountToAssumeRole: &useForward,
	}
}

func cloudAccountResponse(payload sdk.AWSCloudAccountRequest) map[string]any {
	regions := map[string]map[string]int64{}
	for region, instant := range payload.Regions {
		regions[region] = map[string]int64{"testInstant": instant}
	}
	return map[string]any{
		"type":                          "AWS",
		"name":                          payload.Name,
		"collect":                       true,
		"regions":                       regions,
		"regionToProxyServerId":         payload.RegionToProxyServerID,
		"assumeRoleInfos":               payload.AssumeRoleInfos,
		"useForwardAccountToAssumeRole": payload.UseForwardAccountToAssumeRole,
	}
}
