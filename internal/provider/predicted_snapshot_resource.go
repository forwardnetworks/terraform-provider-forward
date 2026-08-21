// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/forwardnetworks/terraform-provider-forward/internal/sdk"
)

var _ resource.Resource = &PredictedSnapshotResource{}

// PredictedSnapshotResource produces a snapshot of the network as it would be
// after a change, without making the change.
//
// It is a resource rather than a data source because it has effects a read must
// not: it opens a change set, stages a draft on it, commits, and leaves a
// snapshot behind. Destroying it discards the change set.
//
// Its point is the id it exposes. The nqe_query and path_analysis data sources
// are snapshot-scoped, so pointing them at this resource asks what Forward
// believes about a network that does not exist yet -- and a check block on the
// answer refuses the apply that would have caused it.
type PredictedSnapshotResource struct {
	providerData *ForwardProviderData
}

// PredictedSnapshotResourceModel stores Terraform state.
type PredictedSnapshotResourceModel struct {
	ID                  types.String `tfsdk:"id"`
	NetworkID           types.String `tfsdk:"network_id"`
	SourceName          types.String `tfsdk:"source_name"`
	BaseSnapshotID      types.String `tfsdk:"base_snapshot_id"`
	TerraformPlanJSON   types.String `tfsdk:"terraform_plan_json"`
	CloudChangesJSON    types.String `tfsdk:"cloud_changes_json"`
	Note                types.String `tfsdk:"note"`
	PollIntervalSeconds types.Int64  `tfsdk:"poll_interval_seconds"`
	TimeoutSeconds      types.Int64  `tfsdk:"timeout_seconds"`
	KeepChangeSet       types.Bool   `tfsdk:"keep_change_set"`

	ChangeSetID types.String `tfsdk:"change_set_id"`
	State       types.String `tfsdk:"state"`
}

func NewPredictedSnapshotResource() resource.Resource {
	return &PredictedSnapshotResource{}
}

func (r *PredictedSnapshotResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_predicted_snapshot"
}

func (r *PredictedSnapshotResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	// Every input replaces rather than updates: a prediction is of one stated
	// change against one base, so changing either makes it a different question
	// and the old snapshot is not an answer to the new one.
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}

	resp.Schema = schema.Schema{
		MarkdownDescription: "Predict the network as it would be after a cloud change, and expose the resulting " +
			"snapshot so snapshot-scoped data sources can be read against it.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Predicted snapshot identifier. Pass this as `snapshot_id` to a data source.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"network_id": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Network identifier. Defaults to the provider's network.",
				PlanModifiers:       replace,
			},
			"source_name": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "Cloud collection source the change applies to. This names a *source*, not a " +
					"device: a cloud source models many devices, none of which is a device by the source's own name.",
				PlanModifiers: replace,
			},
			"base_snapshot_id": schema.StringAttribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: "Snapshot to predict from. Defaults to the most recent *collected* snapshot, " +
					"which is not always the most recent one: every prediction leaves a snapshot behind, and " +
					"predicting from a prediction is refused.",
				PlanModifiers: replace,
			},
			"terraform_plan_json": schema.StringAttribute{
				Optional: true,
				MarkdownDescription: "Output of `terraform show -json`. Forward translates the plan into cloud " +
					"changes and reports what it could not translate. Conflicts with `cloud_changes_json`.",
				PlanModifiers: replace,
			},
			"cloud_changes_json": schema.StringAttribute{
				Optional: true,
				MarkdownDescription: "Cloud changes stated directly, as the JSON body of the cloud-changes " +
					"endpoint. Conflicts with `terraform_plan_json`.",
				PlanModifiers: replace,
			},
			"note": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Note recorded on the commit and the predicted snapshot.",
				PlanModifiers:       replace,
			},
			"poll_interval_seconds": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(10),
				MarkdownDescription: "How often to poll while the prediction processes.",
			},
			"timeout_seconds": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(2400),
				MarkdownDescription: "How long to wait for the prediction to finish processing.",
			},
			"keep_change_set": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(false),
				MarkdownDescription: "Keep the change set when this resource is destroyed. Off by default, so a " +
					"prediction does not leave change sets behind on every apply.",
			},
			"change_set_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Change set the prediction was run from.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"state": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Processing state of the predicted snapshot.",
			},
		},
	}
}

func (r *PredictedSnapshotResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *PredictedSnapshotResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data PredictedSnapshotResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.providerData == nil || r.providerData.Client == nil {
		resp.Diagnostics.AddError("Unconfigured Provider",
			"The provider client was not configured. Re-run terraform init or review provider configuration.")
		return
	}
	client := r.providerData.Client

	plan := data.TerraformPlanJSON.ValueString()
	changes := data.CloudChangesJSON.ValueString()
	switch {
	case plan != "" && changes != "":
		resp.Diagnostics.AddError("Conflicting Change Sources",
			"Set exactly one of terraform_plan_json or cloud_changes_json; stating both leaves it ambiguous which change was predicted.")
		return
	case plan == "" && changes == "":
		resp.Diagnostics.AddError("No Change Stated",
			"Set one of terraform_plan_json or cloud_changes_json. Predicting no change would return the base snapshot under a new id.")
		return
	}

	networkID := data.NetworkID.ValueString()
	if networkID == "" {
		networkID = r.providerData.NetworkID
	}
	if networkID == "" {
		resp.Diagnostics.AddError("Missing Network", "Set network_id on the resource or on the provider.")
		return
	}
	data.NetworkID = types.StringValue(networkID)

	base := data.BaseSnapshotID.ValueString()
	if base == "" {
		latest, err := client.LatestCollectedSnapshot(ctx, networkID)
		if err != nil {
			resp.Diagnostics.AddError("No Base Snapshot", err.Error())
			return
		}
		base = latest.ID
	}
	data.BaseSnapshotID = types.StringValue(base)

	note := data.Note.ValueString()
	if note == "" {
		note = "terraform"
	}
	data.Note = types.StringValue(note)

	changeSet, err := client.CreateChangeSet(ctx, networkID, note, base)
	if err != nil {
		resp.Diagnostics.AddError("Change Set Not Created", err.Error())
		return
	}
	data.ChangeSetID = types.StringValue(changeSet.ID)

	// From here the change set exists, so every failure discards it rather than
	// leaving a draft nobody owns behind.
	fail := func(summary string, err error) {
		if !data.KeepChangeSet.ValueBool() {
			_ = client.DeleteChangeSet(ctx, networkID, changeSet.ID)
		}
		resp.Diagnostics.AddError(summary, err.Error())
	}

	source := data.SourceName.ValueString()
	if plan != "" {
		err = client.StageTerraformPlan(ctx, networkID, changeSet.ID, source, []byte(plan))
	} else {
		var parsed sdk.CloudChanges
		if err = unmarshalCloudChanges(changes, &parsed); err == nil {
			err = client.StageCloudChanges(ctx, networkID, changeSet.ID, source, parsed)
		}
	}
	if err != nil {
		fail("Change Not Staged", err)
		return
	}
	if err := client.CommitChangeSet(ctx, networkID, changeSet.ID, note); err != nil {
		fail("Change Set Not Committed", err)
		return
	}
	predicted, err := client.RunPredict(ctx, networkID, changeSet.ID, note)
	if err != nil {
		fail("Predict Refused", err)
		return
	}
	data.ID = types.StringValue(predicted.ID)
	data.State = types.StringValue(predicted.State)

	// A snapshot still processing has no model to read, so returning here would
	// hand a data source an id it cannot query yet.
	timeout := time.Duration(data.TimeoutSeconds.ValueInt64()) * time.Second
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	details, err := client.AwaitSnapshot(waitCtx, networkID, predicted.ID,
		time.Duration(data.PollIntervalSeconds.ValueInt64())*time.Second)
	if err != nil {
		fail("Prediction Did Not Finish", err)
		return
	}
	data.State = types.StringValue(details.State)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Read refreshes the predicted snapshot's state.
//
// A predicted snapshot is immutable once processed, so this only notices that it
// is gone -- which is a real case: snapshots are archived.
func (r *PredictedSnapshotResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data PredictedSnapshotResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() || r.providerData == nil || r.providerData.Client == nil {
		return
	}
	details, err := r.providerData.Client.GetSnapshot(ctx, data.NetworkID.ValueString(), data.ID.ValueString())
	if err != nil {
		resp.State.RemoveResource(ctx)
		return
	}
	data.State = types.StringValue(details.State)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Update cannot be reached: every input requires replacement.
func (r *PredictedSnapshotResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data PredictedSnapshotResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Delete discards the change set. The predicted snapshot itself is left alone:
// it is a record of what was predicted, and Forward ages snapshots out on its
// own schedule.
func (r *PredictedSnapshotResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data PredictedSnapshotResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() || r.providerData == nil || r.providerData.Client == nil {
		return
	}
	if data.KeepChangeSet.ValueBool() || data.ChangeSetID.ValueString() == "" {
		return
	}
	if err := r.providerData.Client.DeleteChangeSet(ctx,
		data.NetworkID.ValueString(), data.ChangeSetID.ValueString()); err != nil {
		resp.Diagnostics.AddWarning("Change Set Not Discarded",
			fmt.Sprintf("The predicted snapshot is gone from state but change set %s remains: %s",
				data.ChangeSetID.ValueString(), err))
	}
}

// unmarshalCloudChanges decodes the stated changes, refusing silently-empty
// input: a body that parses to no changes at all would predict nothing and
// report success.
func unmarshalCloudChanges(raw string, out *sdk.CloudChanges) error {
	decoder := json.NewDecoder(strings.NewReader(raw))
	// Unknown fields are refused: a misspelled category would otherwise be
	// dropped and the prediction would quietly be of a smaller change.
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		return fmt.Errorf("cloud_changes_json is not a valid cloud changes body: %w", err)
	}
	if len(out.RouteChanges) == 0 && len(out.SecurityRuleChanges) == 0 &&
		len(out.TgwAssociationChanges) == 0 && len(out.TgwPropagationChanges) == 0 &&
		len(out.VpnRouteChanges) == 0 {
		return fmt.Errorf("cloud_changes_json states no changes")
	}
	return nil
}
