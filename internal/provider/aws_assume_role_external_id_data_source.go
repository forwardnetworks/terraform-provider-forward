// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &AWSAssumeRoleExternalIDDataSource{}

// NewAWSAssumeRoleExternalIDDataSource instantiates the AWS assume-role external ID data source.
func NewAWSAssumeRoleExternalIDDataSource() datasource.DataSource {
	return &AWSAssumeRoleExternalIDDataSource{}
}

// AWSAssumeRoleExternalIDDataSource retrieves the Forward-generated AWS external ID.
type AWSAssumeRoleExternalIDDataSource struct {
	providerData *ForwardProviderData
}

type awsAssumeRoleExternalIDDataSourceModel struct {
	NetworkID  types.String `tfsdk:"network_id"`
	ExternalID types.String `tfsdk:"external_id"`
}

func (d *AWSAssumeRoleExternalIDDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_aws_assume_role_external_id"
}

func (d *AWSAssumeRoleExternalIDDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Retrieve the Forward-generated external ID used by AWS assume-role cloud account collection.",
		Attributes: map[string]schema.Attribute{
			"network_id": schema.StringAttribute{
				MarkdownDescription: "Forward network ID. Defaults to the provider network_id when omitted.",
				Optional:            true,
			},
			"external_id": schema.StringAttribute{
				MarkdownDescription: "Forward-generated AWS assume-role external ID.",
				Computed:            true,
			},
		},
	}
}

func (d *AWSAssumeRoleExternalIDDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *AWSAssumeRoleExternalIDDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.providerData == nil {
		resp.Diagnostics.AddError(
			"Client Not Configured",
			"The provider client was not configured. Ensure the provider block is present before using this data source.",
		)
		return
	}

	var data awsAssumeRoleExternalIDDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	networkID := d.providerData.NetworkID
	if !data.NetworkID.IsNull() && !data.NetworkID.IsUnknown() {
		networkID = data.NetworkID.ValueString()
	}
	if networkID == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("network_id"),
			"Missing Network ID",
			"Network ID must be specified either on the provider or data source.",
		)
		return
	}

	externalID, _, err := d.providerData.Client.CloudAccounts.AWSAssumeRoleExternalID(ctx, networkID)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Retrieve AWS External ID", err.Error())
		return
	}
	if externalID == "" {
		resp.Diagnostics.AddError("Empty AWS External ID", "Forward returned an empty AWS assume-role external ID.")
		return
	}

	data.NetworkID = types.StringValue(networkID)
	data.ExternalID = types.StringValue(externalID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
