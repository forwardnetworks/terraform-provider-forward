// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// Copyright (c) HashiCorp, Inc.

package provider

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	diag "github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/forwardnetworks/terraform-provider-forward/internal/sdk"
)

var _ datasource.DataSource = &PathAnalysisDataSource{}

// PathAnalysisDataSource executes path analysis queries.
type PathAnalysisDataSource struct {
	providerData *ForwardProviderData
}

// PathAnalysisModel represents Terraform state.
type PathAnalysisModel struct {
	NetworkID               types.String `tfsdk:"network_id"`
	From                    types.String `tfsdk:"from"`
	SrcIP                   types.String `tfsdk:"src_ip"`
	DstIP                   types.String `tfsdk:"dst_ip"`
	Intent                  types.String `tfsdk:"intent"`
	SnapshotID              types.String `tfsdk:"snapshot_id"`
	IPProto                 types.Int64  `tfsdk:"ip_proto"`
	SrcPort                 types.String `tfsdk:"src_port"`
	DstPort                 types.String `tfsdk:"dst_port"`
	IcmpType                types.Int64  `tfsdk:"icmp_type"`
	TCPFin                  types.Int64  `tfsdk:"tcp_fin"`
	TCPSyn                  types.Int64  `tfsdk:"tcp_syn"`
	TCPRst                  types.Int64  `tfsdk:"tcp_rst"`
	TCPPsh                  types.Int64  `tfsdk:"tcp_psh"`
	TCPAck                  types.Int64  `tfsdk:"tcp_ack"`
	TCPUrg                  types.Int64  `tfsdk:"tcp_urg"`
	AppID                   types.String `tfsdk:"app_id"`
	UserID                  types.String `tfsdk:"user_id"`
	UserGroupID             types.String `tfsdk:"user_group_id"`
	URL                     types.String `tfsdk:"url"`
	IncludeTags             types.Bool   `tfsdk:"include_tags"`
	IncludeNetworkFunctions types.Bool   `tfsdk:"include_network_functions"`
	MaxCandidates           types.Int64  `tfsdk:"max_candidates"`
	MaxResults              types.Int64  `tfsdk:"max_results"`
	MaxReturnResults        types.Int64  `tfsdk:"max_return_path_results"`
	MaxSeconds              types.Int64  `tfsdk:"max_seconds"`

	SrcIPLocationType types.String `tfsdk:"src_ip_location_type"`
	DstIPLocationType types.String `tfsdk:"dst_ip_location_type"`
	TimedOut          types.Bool   `tfsdk:"timed_out"`
	QueryURL          types.String `tfsdk:"query_url"`
	PathsJSON         types.List   `tfsdk:"paths_json"`
	ReturnPathsJSON   types.List   `tfsdk:"return_paths_json"`
	Unrecognized      types.Map    `tfsdk:"unrecognized_values"`
}

func NewPathAnalysisDataSource() datasource.DataSource {
	return &PathAnalysisDataSource{}
}

func (d *PathAnalysisDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_path_analysis"
}

func (d *PathAnalysisDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Execute a path analysis query using the Forward Networks API.",
		Attributes: map[string]schema.Attribute{
			"network_id":                schema.StringAttribute{Required: true, MarkdownDescription: "Network identifier."},
			"from":                      schema.StringAttribute{Optional: true, MarkdownDescription: "Source device name."},
			"src_ip":                    schema.StringAttribute{Optional: true, MarkdownDescription: "Source IP address."},
			"dst_ip":                    schema.StringAttribute{Required: true, MarkdownDescription: "Destination IP address."},
			"intent":                    schema.StringAttribute{Optional: true, MarkdownDescription: "Path analysis intent."},
			"snapshot_id":               schema.StringAttribute{Optional: true},
			"ip_proto":                  schema.Int64Attribute{Optional: true},
			"src_port":                  schema.StringAttribute{Optional: true},
			"dst_port":                  schema.StringAttribute{Optional: true},
			"icmp_type":                 schema.Int64Attribute{Optional: true},
			"tcp_fin":                   schema.Int64Attribute{Optional: true},
			"tcp_syn":                   schema.Int64Attribute{Optional: true},
			"tcp_rst":                   schema.Int64Attribute{Optional: true},
			"tcp_psh":                   schema.Int64Attribute{Optional: true},
			"tcp_ack":                   schema.Int64Attribute{Optional: true},
			"tcp_urg":                   schema.Int64Attribute{Optional: true},
			"app_id":                    schema.StringAttribute{Optional: true},
			"user_id":                   schema.StringAttribute{Optional: true},
			"user_group_id":             schema.StringAttribute{Optional: true},
			"url":                       schema.StringAttribute{Optional: true},
			"include_tags":              schema.BoolAttribute{Optional: true},
			"include_network_functions": schema.BoolAttribute{Optional: true},
			"max_candidates":            schema.Int64Attribute{Optional: true},
			"max_results":               schema.Int64Attribute{Optional: true},
			"max_return_path_results":   schema.Int64Attribute{Optional: true},
			"max_seconds":               schema.Int64Attribute{Optional: true},

			"src_ip_location_type": schema.StringAttribute{Computed: true},
			"dst_ip_location_type": schema.StringAttribute{Computed: true},
			"timed_out":            schema.BoolAttribute{Computed: true},
			"query_url":            schema.StringAttribute{Computed: true},
			"paths_json": schema.ListAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Path results encoded as JSON strings.",
			},
			"return_paths_json": schema.ListAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Return path results encoded as JSON strings.",
			},
			"unrecognized_values": schema.MapAttribute{
				Computed:    true,
				ElementType: types.ListType{ElemType: types.StringType},
			},
		},
	}
}

func (d *PathAnalysisDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	providerData, ok := req.ProviderData.(*ForwardProviderData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected Data Source Configure Type", fmt.Sprintf("Expected *ForwardProviderData, got: %T.", req.ProviderData))
		return
	}

	d.providerData = providerData
}

func (d *PathAnalysisDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.providerData == nil {
		resp.Diagnostics.AddError("Unconfigured Provider", "The provider client was not configured. Re-run terraform init or review provider configuration.")
		return
	}

	var data PathAnalysisModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if data.From.IsNull() && data.SrcIP.IsNull() {
		resp.Diagnostics.AddAttributeError(path.Root("from"), "Invalid configuration", "Either from or src_ip must be supplied.")
		return
	}

	params := buildPathParams(data)
	result, err := d.providerData.Client.SearchPaths(ctx, data.NetworkID.ValueString(), params)
	if err != nil {
		resp.Diagnostics.AddError("Error executing path analysis", err.Error())
		return
	}

	data.SrcIPLocationType = types.StringValue(result.SrcIPLocationType)
	data.DstIPLocationType = types.StringValue(result.DstIPLocationType)
	data.TimedOut = types.BoolValue(result.TimedOut)
	data.QueryURL = types.StringValue(result.QueryURL)

	pathsJSON, diag := marshalPaths(ctx, result.Info.Paths)
	resp.Diagnostics.Append(diag...)
	if resp.Diagnostics.HasError() {
		return
	}
	data.PathsJSON = pathsJSON

	returnJSON, diag := marshalPaths(ctx, result.ReturnPathInfo.Paths)
	resp.Diagnostics.Append(diag...)
	if resp.Diagnostics.HasError() {
		return
	}
	data.ReturnPathsJSON = returnJSON

	unrec, diag := marshalUnrecognized(ctx, result.Unrecognized)
	resp.Diagnostics.Append(diag...)
	if resp.Diagnostics.HasError() {
		return
	}
	data.Unrecognized = unrec

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func buildPathParams(model PathAnalysisModel) sdk.PathSearchParams {
	params := sdk.PathSearchParams{
		From:        stringValue(model.From),
		SrcIP:       stringValue(model.SrcIP),
		DstIP:       model.DstIP.ValueString(),
		Intent:      stringValue(model.Intent),
		SnapshotID:  stringValue(model.SnapshotID),
		SrcPort:     stringValue(model.SrcPort),
		DstPort:     stringValue(model.DstPort),
		AppID:       stringValue(model.AppID),
		UserID:      stringValue(model.UserID),
		UserGroupID: stringValue(model.UserGroupID),
		URL:         stringValue(model.URL),
	}

	setInt := func(dst **int, value types.Int64) {
		if !value.IsNull() && !value.IsUnknown() {
			v := int(value.ValueInt64())
			*dst = &v
		}
	}

	setInt(&params.IPProto, model.IPProto)
	setInt(&params.IcmpType, model.IcmpType)
	setInt(&params.TCPFlags.FIN, model.TCPFin)
	setInt(&params.TCPFlags.SYN, model.TCPSyn)
	setInt(&params.TCPFlags.RST, model.TCPRst)
	setInt(&params.TCPFlags.PSH, model.TCPPsh)
	setInt(&params.TCPFlags.ACK, model.TCPAck)
	setInt(&params.TCPFlags.URG, model.TCPUrg)
	setInt(&params.MaxCandidates, model.MaxCandidates)
	setInt(&params.MaxResults, model.MaxResults)
	setInt(&params.MaxReturnPathResults, model.MaxReturnResults)
	setInt(&params.MaxSeconds, model.MaxSeconds)

	if !model.IncludeTags.IsNull() && !model.IncludeTags.IsUnknown() {
		v := model.IncludeTags.ValueBool()
		params.IncludeTags = &v
	}
	if !model.IncludeNetworkFunctions.IsNull() && !model.IncludeNetworkFunctions.IsUnknown() {
		v := model.IncludeNetworkFunctions.ValueBool()
		params.IncludeNetworkFunctions = &v
	}

	return params
}

func marshalPaths(ctx context.Context, paths []sdk.Path) (types.List, diag.Diagnostics) {
	if len(paths) == 0 {
		return types.ListNull(types.StringType), nil
	}

	values := make([]string, 0, len(paths))
	for _, p := range paths {
		b, err := json.Marshal(p)
		if err != nil {
			return types.ListNull(types.StringType), diag.Diagnostics{diag.NewErrorDiagnostic("Failed to marshal path", err.Error())}
		}
		values = append(values, string(b))
	}

	list, d := types.ListValueFrom(ctx, types.StringType, values)
	return list, d
}

func marshalUnrecognized(ctx context.Context, values sdk.PathUnrecognizedValue) (types.Map, diag.Diagnostics) {
	data := map[string][]string{
		"app_id":        values.AppID,
		"user_id":       values.UserID,
		"user_group_id": values.UserGroupID,
	}
	return types.MapValueFrom(ctx, types.ListType{ElemType: types.StringType}, data)
}

func stringValue(value types.String) string {
	if value.IsNull() || value.IsUnknown() {
		return ""
	}
	return value.ValueString()
}
