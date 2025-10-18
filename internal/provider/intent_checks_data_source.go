// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
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

var _ datasource.DataSource = &IntentChecksDataSource{}

// NewIntentChecksDataSource wires the Forward Enterprise intent checks data source.
func NewIntentChecksDataSource() datasource.DataSource {
	return &IntentChecksDataSource{}
}

// IntentChecksDataSource lists intent checks and their pass/fail status for a snapshot.
type IntentChecksDataSource struct {
	providerData *ForwardProviderData
}

type intentChecksDataSourceModel struct {
	SnapshotID types.String `tfsdk:"snapshot_id"`
	Statuses   types.List   `tfsdk:"status"`
	Priorities types.List   `tfsdk:"priority"`
	Types      types.List   `tfsdk:"type"`

	PassCount    types.Int64       `tfsdk:"pass_count"`
	FailCount    types.Int64       `tfsdk:"fail_count"`
	ErrorCount   types.Int64       `tfsdk:"error_count"`
	TimeoutCount types.Int64       `tfsdk:"timeout_count"`
	Checks       []intentCheckItem `tfsdk:"checks"`
}

type intentCheckItem struct {
	ID                    types.String `tfsdk:"id"`
	Name                  types.String `tfsdk:"name"`
	Status                types.String `tfsdk:"status"`
	Priority              types.String `tfsdk:"priority"`
	Description           types.String `tfsdk:"description"`
	Note                  types.String `tfsdk:"note"`
	Enabled               types.Bool   `tfsdk:"enabled"`
	PerfMonitoringEnabled types.Bool   `tfsdk:"perf_monitoring_enabled"`
	NumViolations         types.Int64  `tfsdk:"num_violations"`
	CreationDateMillis    types.Int64  `tfsdk:"creation_date_millis"`
	ExecutionDateMillis   types.Int64  `tfsdk:"execution_date_millis"`
	ExecutionDuration     types.Int64  `tfsdk:"execution_duration_millis"`
	Tags                  types.List   `tfsdk:"tags"`
}

func (d *IntentChecksDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_intent_checks"
}

func (d *IntentChecksDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Retrieve Forward Enterprise intent checks and their result status for a specific snapshot.",
		Attributes: map[string]schema.Attribute{
			"snapshot_id": schema.StringAttribute{
				MarkdownDescription: "Snapshot identifier to query.",
				Required:            true,
			},
			"status": schema.ListAttribute{
				MarkdownDescription: "Filter checks by status (e.g. PASS, FAIL).",
				Optional:            true,
				ElementType:         types.StringType,
			},
			"priority": schema.ListAttribute{
				MarkdownDescription: "Filter checks by priority (e.g. HIGH).",
				Optional:            true,
				ElementType:         types.StringType,
			},
			"type": schema.ListAttribute{
				MarkdownDescription: "Filter checks by type (e.g. NQE, Predefined).",
				Optional:            true,
				ElementType:         types.StringType,
			},
			"pass_count": schema.Int64Attribute{
				MarkdownDescription: "Number of checks that passed.",
				Computed:            true,
			},
			"fail_count": schema.Int64Attribute{
				MarkdownDescription: "Number of checks that failed.",
				Computed:            true,
			},
			"error_count": schema.Int64Attribute{
				MarkdownDescription: "Number of checks that errored.",
				Computed:            true,
			},
			"timeout_count": schema.Int64Attribute{
				MarkdownDescription: "Number of checks that timed out.",
				Computed:            true,
			},
			"checks": schema.ListNestedAttribute{
				MarkdownDescription: "Intent checks returned by the Forward Enterprise API.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":                        schema.StringAttribute{Computed: true},
						"name":                      schema.StringAttribute{Computed: true},
						"status":                    schema.StringAttribute{Computed: true},
						"priority":                  schema.StringAttribute{Computed: true},
						"description":               schema.StringAttribute{Computed: true},
						"note":                      schema.StringAttribute{Computed: true},
						"enabled":                   schema.BoolAttribute{Computed: true},
						"perf_monitoring_enabled":   schema.BoolAttribute{Computed: true},
						"num_violations":            schema.Int64Attribute{Computed: true},
						"creation_date_millis":      schema.Int64Attribute{Computed: true},
						"execution_date_millis":     schema.Int64Attribute{Computed: true},
						"execution_duration_millis": schema.Int64Attribute{Computed: true},
						"tags": schema.ListAttribute{
							ElementType: types.StringType,
							Computed:    true,
						},
					},
				},
			},
		},
	}
}

func (d *IntentChecksDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *IntentChecksDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.providerData == nil {
		resp.Diagnostics.AddError(
			"Client Not Configured",
			"The provider client was not configured. Ensure the provider block is present before using this data source.",
		)
		return
	}

	var data intentChecksDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if data.SnapshotID.IsNull() || data.SnapshotID.ValueString() == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("snapshot_id"),
			"Missing Snapshot ID",
			"The snapshot_id attribute is required to query intent checks.",
		)
		return
	}

	options, diags := expandCheckListOptions(ctx, data)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	checks, err := d.providerData.Client.ListSnapshotChecks(ctx, data.SnapshotID.ValueString(), options)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Retrieve Intent Checks",
			err.Error(),
		)
		return
	}

	stats := map[string]int64{
		"PASS":    0,
		"FAIL":    0,
		"ERROR":   0,
		"TIMEOUT": 0,
	}

	items := make([]intentCheckItem, 0, len(checks))
	for _, check := range checks {
		item := intentCheckItem{
			ID:                    types.StringValue(check.ID),
			Name:                  stringOrNull(check.Name),
			Status:                stringOrNull(check.Status),
			Priority:              stringOrNull(check.Priority),
			Description:           stringOrNull(check.Description),
			Note:                  stringOrNull(check.Note),
			Enabled:               boolPointerOrNull(check.Enabled),
			PerfMonitoringEnabled: boolPointerOrNull(check.PerfMonitoringEnabled),
			NumViolations:         int64PointerOrNull(check.NumViolations),
			CreationDateMillis:    int64PointerOrNull(check.CreationDateMillis),
			ExecutionDateMillis:   int64PointerOrNull(check.ExecutionDateMillis),
			ExecutionDuration:     int64PointerOrNull(check.ExecutionDuration),
			Tags:                  listOfStrings(check.Tags),
		}

		status := check.Status
		if _, ok := stats[status]; ok {
			stats[status]++
		}

		items = append(items, item)
	}

	data.Checks = items
	data.PassCount = types.Int64Value(stats["PASS"])
	data.FailCount = types.Int64Value(stats["FAIL"])
	data.ErrorCount = types.Int64Value(stats["ERROR"])
	data.TimeoutCount = types.Int64Value(stats["TIMEOUT"])

	tflog.Trace(ctx, "retrieved forward intent checks", map[string]any{"count": len(items)})

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func expandCheckListOptions(ctx context.Context, data intentChecksDataSourceModel) (sdk.CheckListOptions, diag.Diagnostics) {
	var diags diag.Diagnostics
	options := sdk.CheckListOptions{}

	if !data.Statuses.IsNull() && !data.Statuses.IsUnknown() {
		var statuses []string
		d := data.Statuses.ElementsAs(ctx, &statuses, false)
		if d.HasError() {
			diags.Append(d...)
			return options, diags
		}
		options.Statuses = statuses
	}
	if !data.Priorities.IsNull() && !data.Priorities.IsUnknown() {
		var priorities []string
		d := data.Priorities.ElementsAs(ctx, &priorities, false)
		if d.HasError() {
			diags.Append(d...)
			return options, diags
		}
		options.Priorities = priorities
	}
	if !data.Types.IsNull() && !data.Types.IsUnknown() {
		var types []string
		d := data.Types.ElementsAs(ctx, &types, false)
		if d.HasError() {
			diags.Append(d...)
			return options, diags
		}
		options.Types = types
	}

	return options, diags
}

func stringOrNull(value string) types.String {
	if value == "" {
		return types.StringNull()
	}
	return types.StringValue(value)
}

func boolPointerOrNull(value *bool) types.Bool {
	if value == nil {
		return types.BoolNull()
	}
	return types.BoolValue(*value)
}

func int64PointerOrNull(value *int64) types.Int64 {
	if value == nil {
		return types.Int64Null()
	}
	return types.Int64Value(*value)
}

func listOfStrings(values []string) types.List {
	if len(values) == 0 {
		return types.ListNull(types.StringType)
	}
	return types.ListValueMust(types.StringType, stringSliceToValue(values))
}

func stringSliceToValue(values []string) []attr.Value {
	result := make([]attr.Value, 0, len(values))
	for _, v := range values {
		result = append(result, types.StringValue(v))
	}
	return result
}
