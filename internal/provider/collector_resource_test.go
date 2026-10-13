// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// fakeCollectors serves the collector routes the way the appserver does:
// registration answers with the token (authorizationKey) once, the list never
// carries it, and a deleted collector drops out of the list.
type fakeCollectors struct {
	mu         sync.Mutex
	collectors map[string]map[string]string // name -> {id, name, username}
	authKey    string
	deleted    []string
}

func (f *fakeCollectors) handler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/collectors":
			var body struct {
				CollectorName string `json:"collectorName"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.CollectorName == "" {
				http.Error(w, `{"message":"collectorName is required"}`, http.StatusBadRequest)
				return
			}
			if _, ok := f.collectors[body.CollectorName]; ok {
				http.Error(w, `{"message":"Collector already exists"}`, http.StatusBadRequest)
				return
			}
			c := map[string]string{"id": "C101", "name": body.CollectorName, "username": "collector-0a1b2c3d"}
			f.collectors[body.CollectorName] = c
			_ = json.NewEncoder(w).Encode(map[string]string{
				"id": c["id"], "name": c["name"], "username": c["username"], "authorizationKey": f.authKey,
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/collectors":
			list := []map[string]string{}
			for _, c := range f.collectors {
				list = append(list, map[string]string{
					"id": c["id"], "name": c["name"], "username": c["username"], "connectionStatus": "DISCONNECTED",
				})
			}
			_ = json.NewEncoder(w).Encode(list)
		case r.Method == http.MethodDelete && len(r.URL.Path) > len("/api/collectors/"):
			name := r.URL.Path[len("/api/collectors/"):]
			delete(f.collectors, name)
			f.deleted = append(f.deleted, name)
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}
}

func TestCollectorResourceLifecycle(t *testing.T) {
	t.Parallel()

	fake := &fakeCollectors{
		collectors: map[string]map[string]string{},
		authKey:    "collector-0a1b2c3d:00112233445566778899aabbccddeeff",
	}
	server := httptest.NewServer(fake.handler(t))
	defer server.Close()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){
			"forward": providerserver.NewProtocol6WithError(New("test")()),
		},
		CheckDestroy: func(*terraform.State) error {
			fake.mu.Lock()
			defer fake.mu.Unlock()
			if len(fake.deleted) != 1 || fake.deleted[0] != "aws-us-east-1" {
				return fmt.Errorf("destroy should delete collector aws-us-east-1 in Forward once, deleted %v", fake.deleted)
			}
			return nil
		},
		Steps: []resource.TestStep{
			{
				Config: collectorTestConfig(server.URL),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("forward_collector.test", "id", "C101"),
					resource.TestCheckResourceAttr("forward_collector.test", "name", "aws-us-east-1"),
					resource.TestCheckResourceAttr("forward_collector.test", "username", "collector-0a1b2c3d"),
					// The token is what the container's TOKEN takes, and it must
					// survive refreshes even though the list never returns it.
					resource.TestCheckResourceAttr("forward_collector.test", "token",
						"collector-0a1b2c3d:00112233445566778899aabbccddeeff"),
					resource.TestCheckNoResourceAttr("forward_collector.test", "app_server_url"),
				),
			},
			{
				// A second plan after refresh must be empty: Read keeps the token.
				Config:   collectorTestConfig(server.URL),
				PlanOnly: true,
			},
		},
	})
}

// A collector deleted in Forward behind Terraform's back is planned for
// re-creation, not silently kept with a token that no longer works.
func TestCollectorResourceDeletedOutOfBand(t *testing.T) {
	t.Parallel()

	fake := &fakeCollectors{
		collectors: map[string]map[string]string{},
		authKey:    "collector-0a1b2c3d:00112233445566778899aabbccddeeff",
	}
	server := httptest.NewServer(fake.handler(t))
	defer server.Close()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){
			"forward": providerserver.NewProtocol6WithError(New("test")()),
		},
		Steps: []resource.TestStep{
			{
				Config: collectorTestConfig(server.URL),
				Check: func(*terraform.State) error {
					fake.mu.Lock()
					defer fake.mu.Unlock()
					delete(fake.collectors, "aws-us-east-1")
					return nil
				},
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

// A collector deleted and re-registered under the same name is a different
// collector with a different token. Terraform must not adopt it and keep the
// old token, which would leave the deployed collector unable to log in.
func TestCollectorResourceReRegisteredOutOfBand(t *testing.T) {
	t.Parallel()

	fake := &fakeCollectors{
		collectors: map[string]map[string]string{},
		authKey:    "collector-0a1b2c3d:00112233445566778899aabbccddeeff",
	}
	server := httptest.NewServer(fake.handler(t))
	defer server.Close()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){
			"forward": providerserver.NewProtocol6WithError(New("test")()),
		},
		Steps: []resource.TestStep{
			{
				Config: collectorTestConfig(server.URL),
				Check: func(*terraform.State) error {
					fake.mu.Lock()
					defer fake.mu.Unlock()
					fake.collectors["aws-us-east-1"] = map[string]string{
						"id": "C202", "name": "aws-us-east-1", "username": "collector-ffffffff",
					}
					return nil
				},
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

func collectorTestConfig(host string) string {
	return fmt.Sprintf(`
provider "forward" {
  base_url = "%s"
  username = "user"
  password = "pass"
}

resource "forward_collector" "test" {
  name = "aws-us-east-1"
}
`, host)
}
