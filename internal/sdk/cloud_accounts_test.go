// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package sdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientCloudAccounts(t *testing.T) {
	t.Parallel()

	var patched AWSCloudAccountRequest
	var credential AWSCloudAccountCredentialRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/networks/network-1/cloudAccounts":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `[{"type":"AWS","name":"setup-a","collect":true,"regions":{"us-east-1":{"testInstant":123}},"assumeRoleInfos":[{"accountId":"111111111111","accountName":"prod","roleArn":"arn:aws:iam::111111111111:role/Forward","externalId":"Org:55","enabled":true}]}]`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/networks/network-1/cloudAccounts/aws/assumeRole/externalId":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"externalId":"Org:55"}`)
		case r.Method == http.MethodPatch && r.URL.Path == "/api/networks/network-1/cloudAccounts/setup-a":
			if err := json.NewDecoder(r.Body).Decode(&patched); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && r.URL.Path == "/api/networks/network-1/cloudAccounts/setup-a/credential":
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

	client, err := NewClient(context.Background(), Config{BaseURL: server.URL, Username: "user", Password: "pass"})
	if err != nil {
		t.Fatalf("construct client: %v", err)
	}

	accounts, err := client.ListCloudAccounts(context.Background(), "network-1")
	if err != nil {
		t.Fatalf("list cloud accounts: %v", err)
	}
	if len(accounts) != 1 || accounts[0].Name != "setup-a" || len(accounts[0].AssumeRoleInfos) != 1 {
		t.Fatalf("unexpected accounts: %#v", accounts)
	}

	externalID, err := client.AWSAssumeRoleExternalID(context.Background(), "network-1")
	if err != nil {
		t.Fatalf("external id: %v", err)
	}
	if externalID != "Org:55" {
		t.Fatalf("unexpected external ID %q", externalID)
	}

	err = client.UpdateCloudAccount(context.Background(), "network-1", "setup-a", AWSCloudAccountRequest{
		Type:    "AWS",
		Name:    "setup-a",
		Regions: map[string]int64{"us-east-1": 123},
		AssumeRoleInfos: []AWSAssumeRoleInfo{{
			AccountID: "111111111111",
			RoleArn:   "arn:aws:iam::111111111111:role/Forward",
			Enabled:   true,
		}},
		RegionToProxyServerID: map[string]string{},
	})
	if err != nil {
		t.Fatalf("update cloud account: %v", err)
	}
	if patched.Type != "AWS" || patched.Name != "setup-a" || len(patched.AssumeRoleInfos) != 1 {
		t.Fatalf("unexpected patch payload: %#v", patched)
	}

	err = client.UpdateCloudAccountCredential(context.Background(), "network-1", "setup-a", AWSCloudAccountCredentialRequest{
		Type:     "AWS",
		Username: "AKIAEXAMPLE",
		Password: "secret",
	})
	if err != nil {
		t.Fatalf("update cloud account credential: %v", err)
	}
	if credential.Type != "AWS" || credential.Username != "AKIAEXAMPLE" || credential.Password != "secret" {
		t.Fatalf("unexpected credential payload: %#v", credential)
	}
}
