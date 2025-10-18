// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var _ datasource.DataSource = &VersionDataSource{}

// NewVersionDataSource instantiates the version data source.
func NewVersionDataSource() datasource.DataSource {
	return &VersionDataSource{}
}

// VersionDataSource exposes Forward Enterprise build metadata.
type VersionDataSource struct {
	providerData *ForwardProviderData
}

// versionDataSourceModel represents the Terraform state.
type versionDataSourceModel struct {
	Build   types.String `tfsdk:"build"`
	Release types.String `tfsdk:"release"`
	Version types.String `tfsdk:"version"`
}

func (d *VersionDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_version"
}

func (d *VersionDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Retrieve Forward Enterprise API version information.",
		Attributes: map[string]schema.Attribute{
			"build": schema.StringAttribute{
				MarkdownDescription: "Build hash of the Forward Enterprise deployment.",
				Computed:            true,
			},
			"release": schema.StringAttribute{
				MarkdownDescription: "Release identifier of the Forward Enterprise deployment.",
				Computed:            true,
			},
			"version": schema.StringAttribute{
				MarkdownDescription: "API version of the Forward Enterprise deployment.",
				Computed:            true,
			},
		},
	}
}

func (d *VersionDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	providerData, ok := req.ProviderData.(*ForwardProviderData)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *ForwardProviderData, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	d.providerData = providerData
}

func (d *VersionDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.providerData == nil {
		resp.Diagnostics.AddError(
			"Client Not Configured",
			"The provider client was not configured. Ensure the provider block is present before using this data source.",
		)
		return
	}

	version, err := d.providerData.Client.GetVersion(ctx)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Retrieve Version",
			err.Error(),
		)
		return
	}

	state := versionDataSourceModel{
		Build:   types.StringNull(),
		Release: types.StringNull(),
		Version: types.StringNull(),
	}

	if version.Build != "" {
		state.Build = types.StringValue(version.Build)
	}
	if version.Release != "" {
		state.Release = types.StringValue(version.Release)
	}
	if version.Version != "" {
		state.Version = types.StringValue(version.Version)
	}

	tflog.Trace(ctx, "retrieved forward version")

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
