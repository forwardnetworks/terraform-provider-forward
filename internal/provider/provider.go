// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"os"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	schemavalidator "github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/forwardnetworks/terraform-provider-forward/internal/sdk"
)

const (
	envAPIKeyPrimary = "FORWARD_API_KEY"
	envAPIKeyLegacy  = "FORWARD_API_TOKEN"
	envNetworkID     = "FORWARD_NETWORK_ID"
	envBaseURL       = "FORWARD_BASE_URL"
)

var _ provider.Provider = &ForwardProvider{}

// ForwardProviderData houses the configured client and contextual values
// that resources and data sources will require.
type ForwardProviderData struct {
	Client    *sdk.Client
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
	APIKey    types.String `tfsdk:"api_key"`
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
				MarkdownDescription: "Base URL for the Forward Networks API, for example `https://demo.forwardnetworks.com`.",
				Required:            true,
				Validators: []schemavalidator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"api_key": schema.StringAttribute{
				MarkdownDescription: "API key used to authenticate requests. Marked sensitive and typically sourced from the `FORWARD_API_KEY` environment variable.",
				Required:            true,
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
				Required:            true,
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
	apiKey := ""
	if !data.APIKey.IsNull() {
		apiKey = data.APIKey.ValueString()
	}
	if apiKey == "" {
		apiKey = os.Getenv(envAPIKeyPrimary)
	}
	if apiKey == "" {
		apiKey = os.Getenv(envAPIKeyLegacy)
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

	if apiKey == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("api_key"),
			"Missing API Key",
			"The provider cannot create the Forward Networks client because the `api_key` attribute is empty. "+
				"Set the `api_key` attribute or the `FORWARD_API_KEY` environment variable.",
		)
		return
	}

	if networkID == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("network_id"),
			"Missing Network ID",
			"The provider cannot create the Forward Networks client because the `network_id` attribute is empty. "+
				"Set the `network_id` attribute in the Terraform configuration or define the `FORWARD_NETWORK_ID` environment variable.",
		)
		return
	}

	client, err := sdk.NewClient(ctx, sdk.Config{
		BaseURL:  baseURL,
		APIKey:   apiKey,
		Insecure: insecure,
		UserAgent: fmt.Sprintf(
			"terraform-provider-forward/%s",
			p.version,
		),
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
	}
}

func (p *ForwardProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewVersionDataSource,
		NewSnapshotsDataSource,
		NewIntentChecksDataSource,
		NewNqeQueryDataSource,
		NewPathAnalysisDataSource,
	}
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &ForwardProvider{
			version: version,
		}
	}
}
