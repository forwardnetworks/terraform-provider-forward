// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// Copyright (c) HashiCorp, Inc.

package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/forwardnetworks/terraform-provider-forward/internal/sdk"
)

var _ resource.Resource = &SnapshotResource{}
var _ resource.ResourceWithImportState = &SnapshotResource{}

// SnapshotResource manages Forward snapshot lifecycle.
type SnapshotResource struct {
	providerData *ForwardProviderData
}

// SnapshotResourceModel stores Terraform state.
type SnapshotResourceModel struct {
	ID                  types.String `tfsdk:"id"`
	NetworkID           types.String `tfsdk:"network_id"`
	Note                types.String `tfsdk:"note"`
	WaitForProcessed    types.Bool   `tfsdk:"wait_for_processed"`
	PollIntervalSeconds types.Int64  `tfsdk:"poll_interval_seconds"`
	TimeoutSeconds      types.Int64  `tfsdk:"timeout_seconds"`

	State              types.String `tfsdk:"state"`
	CreationDateMillis types.Int64  `tfsdk:"creation_date_millis"`
	ProcessedAtMillis  types.Int64  `tfsdk:"processed_at_millis"`
	RestoredAtMillis   types.Int64  `tfsdk:"restored_at_millis"`
}

func NewSnapshotResource() resource.Resource {
	return &SnapshotResource{}
}

func (r *SnapshotResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_snapshot"
}

func (r *SnapshotResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manage Forward Enterprise snapshots (capture, poll, and archive).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Snapshot identifier assigned by Forward Enterprise.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"network_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Network identifier associated with the snapshot.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"note": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Optional note attached to the snapshot.",
			},
			"wait_for_processed": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Wait for the snapshot to reach PROCESSED state before completing create.",
				Default:             booldefault.StaticBool(true),
			},
			"poll_interval_seconds": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Interval in seconds between polling attempts when wait_for_processed is true.",
				Default:             int64default.StaticInt64(10),
			},
			"timeout_seconds": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Maximum seconds to wait for the snapshot to reach PROCESSED.",
				Default:             int64default.StaticInt64(600),
			},
			"state": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Current snapshot state.",
			},
			"creation_date_millis": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Snapshot creation timestamp (milliseconds).",
			},
			"processed_at_millis": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Snapshot processed timestamp (milliseconds).",
			},
			"restored_at_millis": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Snapshot restored timestamp (milliseconds).",
			},
		},
	}
}

func (r *SnapshotResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *SnapshotResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	if r.providerData == nil {
		resp.Diagnostics.AddError("Unconfigured Provider", "The provider client was not configured. Re-run terraform init or review provider configuration.")
		return
	}

	var plan SnapshotResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	request := sdk.SnapshotCreateRequest{}
	if !plan.Note.IsNull() && !plan.Note.IsUnknown() {
		request.Note = plan.Note.ValueString()
	}

	snapshot, err := r.providerData.Client.CreateSnapshot(ctx, plan.NetworkID.ValueString(), request)
	if err != nil {
		resp.Diagnostics.AddError("Error creating snapshot", err.Error())
		return
	}

	plan.ID = types.StringValue(snapshot.ID)
	updateSnapshotState(&plan, snapshot)

	wait := !plan.WaitForProcessed.IsNull() && plan.WaitForProcessed.ValueBool()
	if wait {
		pollInterval := defaultInt(plan.PollIntervalSeconds, 10)
		timeout := defaultInt(plan.TimeoutSeconds, 600)
		if pollErr := r.waitForProcessed(ctx, plan.NetworkID.ValueString(), snapshot.ID, time.Duration(pollInterval)*time.Second, time.Duration(timeout)*time.Second, &plan); pollErr != nil {
			resp.Diagnostics.AddError("Error waiting for snapshot", pollErr.Error())
			return
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *SnapshotResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	if r.providerData == nil {
		resp.Diagnostics.AddError("Unconfigured Provider", "The provider client was not configured. Re-run terraform init or review provider configuration.")
		return
	}

	var state SnapshotResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	snapshot, err := r.providerData.Client.GetSnapshot(ctx, state.NetworkID.ValueString(), state.ID.ValueString())
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading snapshot", err.Error())
		return
	}

	updateSnapshotState(&state, snapshot)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *SnapshotResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// All meaningful fields require recreation. Nothing to do.
	var plan SnapshotResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *SnapshotResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	if r.providerData == nil {
		resp.Diagnostics.AddError("Unconfigured Provider", "The provider client was not configured. Re-run terraform init or review provider configuration.")
		return
	}

	var state SnapshotResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.providerData.Client.DeleteSnapshot(ctx, state.ID.ValueString()); err != nil && !isNotFoundError(err) {
		resp.Diagnostics.AddError("Error deleting snapshot", err.Error())
	}
}

func (r *SnapshotResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, "/")
	if len(parts) != 2 {
		resp.Diagnostics.AddError("Invalid import format", "Use: network_id/snapshot_id")
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("network_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[1])...)
}

func (r *SnapshotResource) waitForProcessed(ctx context.Context, networkID, snapshotID string, interval, timeout time.Duration, state *SnapshotResourceModel) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	timeoutChan := time.After(timeout)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeoutChan:
			return errors.New("snapshot processing timed out")
		case <-ticker.C:
			snapshot, err := r.providerData.Client.GetSnapshot(ctx, networkID, snapshotID)
			if err != nil {
				if strings.Contains(strings.ToLower(err.Error()), "not found") {
					return err
				}
				continue
			}

			updateSnapshotState(state, snapshot)
			if strings.EqualFold(snapshot.State, "PROCESSED") {
				return nil
			}
			if strings.EqualFold(snapshot.State, "FAILED") {
				return fmt.Errorf("snapshot %s failed", snapshotID)
			}
		}
	}
}

func updateSnapshotState(model *SnapshotResourceModel, snapshot *sdk.SnapshotDetails) {
	model.State = stringOrNullValue(snapshot.State)
	if snapshot.CreationDateMillis != nil {
		model.CreationDateMillis = types.Int64Value(*snapshot.CreationDateMillis)
	} else {
		model.CreationDateMillis = types.Int64Null()
	}
	if snapshot.ProcessedAtMillis != nil {
		model.ProcessedAtMillis = types.Int64Value(*snapshot.ProcessedAtMillis)
	} else {
		model.ProcessedAtMillis = types.Int64Null()
	}
	if snapshot.RestoredAtMillis != nil {
		model.RestoredAtMillis = types.Int64Value(*snapshot.RestoredAtMillis)
	} else {
		model.RestoredAtMillis = types.Int64Null()
	}
}

func defaultInt(value types.Int64, fallback int64) int64 {
	if value.IsNull() || value.IsUnknown() {
		return fallback
	}
	return value.ValueInt64()
}

func stringOrNullValue(value string) types.String {
	if strings.TrimSpace(value) == "" {
		return types.StringNull()
	}
	return types.StringValue(value)
}
