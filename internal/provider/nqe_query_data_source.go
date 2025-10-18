// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/forwardnetworks/terraform-provider-forward/internal/sdk"
)

var _ datasource.DataSource = &NqeQueryDataSource{}

// NewNqeQueryDataSource instantiates the NQE query data source.
func NewNqeQueryDataSource() datasource.DataSource {
	return &NqeQueryDataSource{}
}

// NqeQueryDataSource executes an NQE query and returns the raw results.
type NqeQueryDataSource struct {
	providerData *ForwardProviderData
}

type nqeQueryDataSourceModel struct {
	SnapshotID types.String `tfsdk:"snapshot_id"`
	NetworkID  types.String `tfsdk:"network_id"`
	Query      types.String `tfsdk:"query"`
	QueryID    types.String `tfsdk:"query_id"`
	CommitID   types.String `tfsdk:"commit_id"`
	Parameters types.Map    `tfsdk:"parameters"`
	Limit      types.Int64  `tfsdk:"limit"`
	Offset     types.Int64  `tfsdk:"offset"`

	ResultSnapshotID types.String `tfsdk:"result_snapshot_id"`
	TotalItems       types.Int64  `tfsdk:"total_items"`
	ItemsJSON        types.List   `tfsdk:"items_json"`
}

func (d *NqeQueryDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_nqe_query"
}

func (d *NqeQueryDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Execute a Forward Enterprise NQE query against a snapshot or network.",
		Attributes: map[string]schema.Attribute{
			"snapshot_id": schema.StringAttribute{
				MarkdownDescription: "Snapshot ID to query. If omitted, defaults to the provider snapshot (latest processed) when network_id is supplied.",
				Optional:            true,
			},
			"network_id": schema.StringAttribute{
				MarkdownDescription: "Network ID to query. Defaults to the provider network_id when omitted.",
				Optional:            true,
			},
			"query": schema.StringAttribute{
				MarkdownDescription: "Inline NQE query to execute.",
				Optional:            true,
			},
			"query_id": schema.StringAttribute{
				MarkdownDescription: "Identifier of a stored NQE query in the Forward Enterprise library.",
				Optional:            true,
			},
			"commit_id": schema.StringAttribute{
				MarkdownDescription: "Specific query commit ID to execute when using query_id.",
				Optional:            true,
			},
			"parameters": schema.MapAttribute{
				MarkdownDescription: "Parameter values to supply to the query (JSON-encoded).",
				ElementType:         types.StringType,
				Optional:            true,
			},
			"limit": schema.Int64Attribute{
				MarkdownDescription: "Limit number of results returned.",
				Optional:            true,
			},
			"offset": schema.Int64Attribute{
				MarkdownDescription: "Offset into the result set.",
				Optional:            true,
			},
			"result_snapshot_id": schema.StringAttribute{
				MarkdownDescription: "Snapshot ID used for query execution.",
				Computed:            true,
			},
			"total_items": schema.Int64Attribute{
				MarkdownDescription: "Total items reported by the Forward Enterprise API.",
				Computed:            true,
			},
			"items_json": schema.ListAttribute{
				MarkdownDescription: "Query results serialized as JSON strings.",
				ElementType:         types.StringType,
				Computed:            true,
			},
		},
	}
}

func (d *NqeQueryDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *NqeQueryDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.providerData == nil {
		resp.Diagnostics.AddError(
			"Client Not Configured",
			"The provider client was not configured. Ensure the provider block is present before using this data source.",
		)
		return
	}

	var data nqeQueryDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	networkID := d.providerData.NetworkID
	if !data.NetworkID.IsNull() && !data.NetworkID.IsUnknown() {
		networkID = data.NetworkID.ValueString()
	}

	if networkID == "" && (data.SnapshotID.IsNull() || data.SnapshotID.ValueString() == "") {
		resp.Diagnostics.AddError(
			"Missing Network Or Snapshot",
			"Provide either network_id or snapshot_id to execute an NQE query.",
		)
		return
	}

	if (data.Query.IsNull() || data.Query.ValueString() == "") && (data.QueryID.IsNull() || data.QueryID.ValueString() == "") {
		resp.Diagnostics.AddAttributeError(
			path.Root("query"),
			"Missing Query",
			"Either query or query_id must be provided to execute an NQE query.",
		)
		return
	}

	reqBody, diags := expandNqeRequest(ctx, data)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := d.providerData.Client.RunNQEQuery(ctx, networkID, stringOrEmpty(data.SnapshotID), reqBody)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Execute NQE Query",
			err.Error(),
		)
		return
	}

	items := make([]attr.Value, 0, len(result.Items))
	for _, raw := range result.Items {
		encoded := json.RawMessage(raw)
		if len(encoded) == 0 {
			items = append(items, types.StringValue("{}"))
			continue
		}
		items = append(items, types.StringValue(string(encoded)))
	}

	state := nqeQueryDataSourceModel{
		SnapshotID:       data.SnapshotID,
		NetworkID:        types.StringValue(networkID),
		Query:            data.Query,
		QueryID:          data.QueryID,
		CommitID:         data.CommitID,
		Parameters:       data.Parameters,
		Limit:            data.Limit,
		Offset:           data.Offset,
		ResultSnapshotID: types.StringValue(result.SnapshotID),
		ItemsJSON:        types.ListNull(types.StringType),
		TotalItems:       types.Int64Null(),
	}

	if len(items) > 0 {
		state.ItemsJSON = types.ListValueMust(types.StringType, items)
	} else {
		state.ItemsJSON = types.ListValueMust(types.StringType, []attr.Value{})
	}

	if result.TotalNumItems != nil {
		state.TotalItems = types.Int64Value(*result.TotalNumItems)
	} else {
		state.TotalItems = types.Int64Value(int64(len(result.Items)))
	}

	if result.SnapshotID == "" {
		state.ResultSnapshotID = types.StringNull()
	}

	tflog.Trace(ctx, "executed forward nqe query", map[string]any{"items": len(result.Items)})

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func expandNqeRequest(ctx context.Context, data nqeQueryDataSourceModel) (sdk.NqeQueryRequest, diag.Diagnostics) {
	var diags diag.Diagnostics
	req := sdk.NqeQueryRequest{}

	if !data.Query.IsNull() && !data.Query.IsUnknown() {
		query := data.Query.ValueString()
		req.Query = &query
	}
	if !data.QueryID.IsNull() && !data.QueryID.IsUnknown() {
		queryID := data.QueryID.ValueString()
		req.QueryID = &queryID
	}
	if !data.CommitID.IsNull() && !data.CommitID.IsUnknown() {
		commit := data.CommitID.ValueString()
		req.CommitID = &commit
	}

	if !data.Parameters.IsNull() && !data.Parameters.IsUnknown() {
		params := map[string]string{}
		d := data.Parameters.ElementsAs(ctx, &params, false)
		if d.HasError() {
			diags.Append(d...)
			return req, diags
		}
		req.Parameters = map[string]any{}
		for k, v := range params {
			var decoded any
			if err := json.Unmarshal([]byte(v), &decoded); err != nil {
				diags.AddAttributeError(
					path.Root("parameters").AtMapKey(k),
					"Invalid Parameter JSON",
					fmt.Sprintf("Parameter %q must be valid JSON: %s", k, err),
				)
				return req, diags
			}
			req.Parameters[k] = decoded
		}
	}

	var limitPtr *int
	if !data.Limit.IsNull() && !data.Limit.IsUnknown() {
		val := int(data.Limit.ValueInt64())
		if val < 0 {
			diags.AddAttributeError(
				path.Root("limit"),
				"Invalid Limit",
				"limit must be zero or positive.",
			)
			return req, diags
		}
		limitPtr = &val
	}

	var offsetPtr *int
	if !data.Offset.IsNull() && !data.Offset.IsUnknown() {
		val := int(data.Offset.ValueInt64())
		if val < 0 {
			diags.AddAttributeError(
				path.Root("offset"),
				"Invalid Offset",
				"offset must be zero or positive.",
			)
			return req, diags
		}
		offsetPtr = &val
	}

	if limitPtr != nil || offsetPtr != nil {
		req.QueryOptions = &sdk.NqeQueryOptions{Limit: limitPtr, Offset: offsetPtr}
	}

	return req, diags
}

func stringOrEmpty(value types.String) string {
	if value.IsNull() || value.IsUnknown() {
		return ""
	}
	return value.ValueString()
}
