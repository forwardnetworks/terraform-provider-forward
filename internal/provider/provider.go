// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	schemavalidator "github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	forward "github.com/forwardnetworks/forward-go-sdk"
)

const (
	envUsername   = "FORWARD_USERNAME"
	envPassword   = "FORWARD_PASSWORD"
	envUserLegacy = "FWD_USER"
	envPassLegacy = "FWD_PASS"
	envNetworkID  = "FORWARD_NETWORK_ID"
	envBaseURL    = "FORWARD_BASE_URL"
)

var _ provider.Provider = &ForwardProvider{}

// ForwardProviderData houses the configured client and contextual values
// that resources and data sources will require.
type ForwardProviderData struct {
	Client    *forward.Client
	NetworkID string
}

// ForwardProvider defines the provider implementation.
type ForwardProvider struct {
	// version is set to the provider version on release, "dev" when the
	// provider is built and ran locally, and "test" when running acceptance
	// testing.
	version string
}

// ForwardProviderModel describes the provider data model.
type ForwardProviderModel struct {
	BaseURL   types.String `tfsdk:"base_url"`
	Username  types.String `tfsdk:"username"`
	Password  types.String `tfsdk:"password"`
	Insecure  types.Bool   `tfsdk:"insecure"`
	NetworkID types.String `tfsdk:"network_id"`
}

func (p *ForwardProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "forward"
	resp.Version = p.version
}

func (p *ForwardProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Use the Forward Enterprise provider to interact with the Forward Networks platform APIs.",
		Attributes: map[string]schema.Attribute{
			"base_url": schema.StringAttribute{
				MarkdownDescription: "Base URL for the Forward Networks API, for example `https://fwd.app`.",
				Optional:            true,
				Validators: []schemavalidator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"username": schema.StringAttribute{
				MarkdownDescription: "Forward username for Basic authentication. Typically sourced from `FORWARD_USERNAME` or `FWD_USER`.",
				Optional:            true,
				Validators: []schemavalidator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"password": schema.StringAttribute{
				MarkdownDescription: "Forward password for Basic authentication. Typically sourced from `FORWARD_PASSWORD` or `FWD_PASS`.",
				Optional:            true,
				Sensitive:           true,
				Validators: []schemavalidator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"insecure": schema.BoolAttribute{
				MarkdownDescription: "Disable TLS certificate verification (not recommended). Useful for testing against development appliances.",
				Optional:            true,
			},
			"network_id": schema.StringAttribute{
				MarkdownDescription: "Default Forward Enterprise Network ID used by resources and data sources when an explicit network is not provided.",
				Optional:            true,
				Validators: []schemavalidator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
		},
	}
}

func (p *ForwardProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var data ForwardProviderModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	baseURL := ""
	if !data.BaseURL.IsNull() {
		baseURL = data.BaseURL.ValueString()
	}
	if baseURL == "" {
		baseURL = os.Getenv(envBaseURL)
	}
	username := ""
	if !data.Username.IsNull() {
		username = data.Username.ValueString()
	}
	if username == "" {
		username = os.Getenv(envUsername)
	}
	if username == "" {
		username = os.Getenv(envUserLegacy)
	}

	password := ""
	if !data.Password.IsNull() {
		password = data.Password.ValueString()
	}
	if password == "" {
		password = os.Getenv(envPassword)
	}
	if password == "" {
		password = os.Getenv(envPassLegacy)
	}

	insecure := false
	if !data.Insecure.IsNull() {
		insecure = data.Insecure.ValueBool()
	}

	networkID := ""
	if !data.NetworkID.IsNull() {
		networkID = data.NetworkID.ValueString()
	}
	if networkID == "" {
		networkID = os.Getenv(envNetworkID)
	}

	if baseURL == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("base_url"),
			"Missing Base URL",
			"The provider cannot create the Forward Networks client because the `base_url` attribute is empty. "+
				"Set the `base_url` attribute in the Terraform configuration or define the `FORWARD_BASE_URL` environment variable.",
		)
		return
	}

	if username == "" || password == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("username"),
			"Missing Authentication",
			"The provider cannot create the Forward Networks client because no authentication method was supplied. "+
				"Set both `username` and `password`. Environment fallbacks are `FORWARD_USERNAME`/`FORWARD_PASSWORD` or `FWD_USER`/`FWD_PASS`.",
		)
		return
	}

	// A network ID is a convenience default, not a precondition. Requiring one
	// here made it impossible to write the configuration that creates a
	// network: there was nothing to name until Terraform had run. Resources
	// that need a network say so themselves when neither the resource nor the
	// provider supplies one.

	client, err := forward.NewClient(forward.Config{
		BaseURL:            baseURL,
		Username:           username,
		Password:           password,
		InsecureSkipVerify: insecure,
		NetworkID:          networkID,
		UserAgent: fmt.Sprintf(
			"terraform-provider-forward/%s",
			p.version,
		),
		// A plan or apply that dies on one 502 from a load balancer is worse
		// than one that waits a moment, and Terraform has no way to resume a
		// half-finished operation.
		Retry: forward.RetryPolicy{MaxAttempts: 3, Delay: 500 * time.Millisecond},
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Configure Forward Networks Client",
			err.Error(),
		)
		return
	}

	providerData := &ForwardProviderData{
		Client:    client,
		NetworkID: networkID,
	}

	resp.DataSourceData = providerData
	resp.ResourceData = providerData
}

func (p *ForwardProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewIntentCheckResource,
		NewNQEQueryResource,
		NewSnapshotResource,
		NewPredictedSnapshotResource,
		NewAWSCloudAccountResource,
		NewNQELibraryQueryResource,
		NewNetworkResource,
		NewProxyResource,
		NewCollectorAttachmentResource,
	}
}

func (p *ForwardProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewVersionDataSource,
		NewSnapshotsDataSource,
		NewIntentChecksDataSource,
		NewNqeQueryDataSource,
		NewPathAnalysisDataSource,
		NewAWSAssumeRoleExternalIDDataSource,
		NewAWSOrganizationAccountsDataSource,
	}
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &ForwardProvider{
			version: version,
		}
	}
}
