// Copyright (c) HashiCorp, Inc.

package provider

import (
	"context"
	"encoding/json"
	"errors"

	"fmt"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

	"github.com/forwardnetworks/terraform-provider-forward/internal/sdk"
)

var _ resource.Resource = &IntentCheckResource{}
var _ resource.ResourceWithImportState = &IntentCheckResource{}

// IntentCheckResource manages Forward Enterprise intent checks bound to a snapshot.
type IntentCheckResource struct {
	providerData *ForwardProviderData
}

// IntentCheckResourceModel maps Terraform schema data.
type IntentCheckResourceModel struct {
	ID                    types.String `tfsdk:"id"`
	SnapshotID            types.String `tfsdk:"snapshot_id"`
	Persistent            types.Bool   `tfsdk:"persistent"`
	DefinitionJSON        types.String `tfsdk:"definition_json"`
	Name                  types.String `tfsdk:"name"`
	Note                  types.String `tfsdk:"note"`
	Enabled               types.Bool   `tfsdk:"enabled"`
	PerfMonitoringEnabled types.Bool   `tfsdk:"perf_monitoring_enabled"`
	Priority              types.String `tfsdk:"priority"`
	Tags                  types.List   `tfsdk:"tags"`

	Status            types.String `tfsdk:"status"`
	NumViolations     types.Int64  `tfsdk:"num_violations"`
	ExecutionDateMs   types.Int64  `tfsdk:"execution_date_millis"`
	ExecutionDuration types.Int64  `tfsdk:"execution_duration_millis"`
}

func NewIntentCheckResource() resource.Resource {
	return &IntentCheckResource{}
}

func (r *IntentCheckResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_intent_check"
}

func (r *IntentCheckResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manage Forward Enterprise intent checks against a specific snapshot.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Identifier assigned by Forward Enterprise for the intent check.",
			},
			"snapshot_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Snapshot identifier the check is evaluated against.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"persistent": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Whether the intent check should persist to future snapshots.",
				Default:             booldefault.StaticBool(true),
			},
			"definition_json": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Raw JSON payload describing the Forward intent check definition (as expected by the Forward API).",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Optional human readable name for the intent check.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"note": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Optional descriptive note stored with the check.",
			},
			"enabled": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Whether the intent check should be enabled when created.",
			},
			"perf_monitoring_enabled": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Enable performance monitoring (supported for existential checks only).",
			},
			"priority": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Intent check priority (NOT_SET, LOW, MEDIUM, HIGH).",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"tags": schema.ListAttribute{
				Optional:            true,
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Tags assigned to the intent check.",
				Default:             listdefault.StaticValue(types.ListNull(types.StringType)),
			},
			"status": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Last known Forward Enterprise status for the check.",
			},
			"num_violations": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Number of violations detected by the check.",
			},
			"execution_date_millis": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Execution timestamp (milliseconds since epoch).",
			},
			"execution_duration_millis": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Execution duration in milliseconds.",
			},
		},
	}
}

func (r *IntentCheckResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	providerData, ok := req.ProviderData.(*ForwardProviderData)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *ForwardProviderData, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	r.providerData = providerData
}

func (r *IntentCheckResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	if r.providerData == nil {
		resp.Diagnostics.AddError(
			"Unconfigured Provider",
			"The provider client was not configured. Re-run terraform init or review provider configuration.",
		)
		return
	}

	var plan IntentCheckResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	definition, diags := parseCheckDefinition(plan.DefinitionJSON)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	reqBody := sdk.NewCheckRequest{
		Definition:            definition,
		Enabled:               boolPointer(plan.Enabled),
		Name:                  stringOrEmpty(plan.Name),
		Note:                  stringOrEmpty(plan.Note),
		PerfMonitoringEnabled: boolPointer(plan.PerfMonitoringEnabled),
		Priority:              stringOrEmpty(plan.Priority),
		Tags:                  stringList(plan.Tags),
	}

	persistent := boolPointer(plan.Persistent)

	result, err := r.providerData.Client.AddSnapshotCheck(ctx, plan.SnapshotID.ValueString(), reqBody, persistent)
	if err != nil {
		resp.Diagnostics.AddError("Error creating intent check", err.Error())
		return
	}

	plan.ID = types.StringValue(result.ID)
	setCheckState(ctx, &plan, result)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *IntentCheckResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	if r.providerData == nil {
		resp.Diagnostics.AddError(
			"Unconfigured Provider",
			"The provider client was not configured. Re-run terraform init or review provider configuration.",
		)
		return
	}

	var state IntentCheckResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.providerData.Client.GetSnapshotCheck(ctx, state.SnapshotID.ValueString(), state.ID.ValueString())
	if err != nil {
		if isNotFoundError(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading intent check", err.Error())
		return
	}

	setCheckState(ctx, &state, &result.CheckResult)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *IntentCheckResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// All mutable attributes require replacement. Nothing to do here.
	var plan IntentCheckResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *IntentCheckResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	if r.providerData == nil {
		resp.Diagnostics.AddError(
			"Unconfigured Provider",
			"The provider client was not configured. Re-run terraform init or review provider configuration.",
		)
		return
	}

	var state IntentCheckResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.providerData.Client.DeactivateSnapshotCheck(ctx, state.SnapshotID.ValueString(), state.ID.ValueString())
	if err != nil && !isNotFoundError(err) {
		resp.Diagnostics.AddError("Error deleting intent check", err.Error())
	}
}

func (r *IntentCheckResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func parseCheckDefinition(definition types.String) (sdk.CheckDefinition, diag.Diagnostics) {
	var diags diag.Diagnostics
	if definition.IsNull() || definition.IsUnknown() {
		diags.AddAttributeError(path.Root("definition_json"), "Missing Definition", "definition_json must be provided.")
		return nil, diags
	}

	var payload sdk.CheckDefinition
	if err := json.Unmarshal([]byte(definition.ValueString()), &payload); err != nil {
		diags.AddAttributeError(path.Root("definition_json"), "Invalid Definition JSON", err.Error())
		return nil, diags
	}

	return payload, diags
}

func setCheckState(_ context.Context, model *IntentCheckResourceModel, result *sdk.CheckResult) {
	if result == nil {
		return
	}

	model.Status = stringOrNull(result.Status)
	model.Name = stringOrNull(result.Name)
	model.Note = stringOrNull(result.Note)

	if result.Enabled != nil {
		model.Enabled = types.BoolValue(*result.Enabled)
	} else {
		model.Enabled = types.BoolNull()
	}
	if result.PerfMonitoringEnabled != nil {
		model.PerfMonitoringEnabled = types.BoolValue(*result.PerfMonitoringEnabled)
	} else {
		model.PerfMonitoringEnabled = types.BoolNull()
	}

	model.Priority = stringOrNull(result.Priority)
	model.Tags = stringSliceToList(result.Tags)

	if result.NumViolations != nil {
		model.NumViolations = types.Int64Value(*result.NumViolations)
	} else {
		model.NumViolations = types.Int64Null()
	}
	if result.ExecutionDateMillis != nil {
		model.ExecutionDateMs = types.Int64Value(*result.ExecutionDateMillis)
	} else {
		model.ExecutionDateMs = types.Int64Null()
	}
	if result.ExecutionDuration != nil {
		model.ExecutionDuration = types.Int64Value(*result.ExecutionDuration)
	} else {
		model.ExecutionDuration = types.Int64Null()
	}
}

func boolPointer(value types.Bool) *bool {
	if value.IsNull() || value.IsUnknown() {
		return nil
	}
	v := value.ValueBool()
	return &v
}

func attrStringValue(value types.String) string {
	if value.IsNull() || value.IsUnknown() {
		return ""
	}
	return value.ValueString()
}

func stringSliceToList(values []string) types.List {
	if len(values) == 0 {
		return types.ListNull(types.StringType)
	}

	elements := make([]attr.Value, 0, len(values))
	for _, v := range values {
		elements = append(elements, types.StringValue(v))
	}

	return types.ListValueMust(types.StringType, elements)
}

func stringList(list types.List) []string {
	if list.IsNull() || list.IsUnknown() {
		return nil
	}

	var values []string
	for _, v := range list.Elements() {
		if str, ok := v.(basetypes.StringValue); ok {
			values = append(values, str.ValueString())
		}
	}
	return values
}

func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, context.Canceled) || strings.Contains(strings.ToLower(err.Error()), "not found") || strings.Contains(err.Error(), "404")
}
