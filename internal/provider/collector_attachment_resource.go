// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	forward "github.com/forwardnetworks/forward-go-sdk"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &CollectorAttachmentResource{}
	_ resource.ResourceWithImportState = &CollectorAttachmentResource{}
)

// CollectorAttachmentResource binds a collector to a network.
//
// A network without one cannot collect: Forward refuses with "has no
// configured Collector". Registering a collector is a separate act -- it
// enrolls itself with credentials -- and this says which network it works for.
type CollectorAttachmentResource struct {
	providerData *ForwardProviderData
}

type collectorAttachmentResourceModel struct {
	ID        types.String `tfsdk:"id"`
	NetworkID types.String `tfsdk:"network_id"`
	Username  types.String `tfsdk:"username"`
	Name      types.String `tfsdk:"collector_name"`
}

func NewCollectorAttachmentResource() resource.Resource { return &CollectorAttachmentResource{} }

func (r *CollectorAttachmentResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_collector_attachment"
}

func (r *CollectorAttachmentResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Binds an already-registered collector to a network. Without one, a network cannot " +
			"collect at all.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"network_id": schema.StringAttribute{
				Required:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"username": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The collector's username, as it enrolled.",
			},
			"collector_name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Name Forward knows the attached collector by.",
			},
		},
	}
}

func (r *CollectorAttachmentResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	providerData, ok := req.ProviderData.(*ForwardProviderData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *ForwardProviderData, got: %T.", req.ProviderData))
		return
	}
	r.providerData = providerData
}

func (r *CollectorAttachmentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan collectorAttachmentResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() || r.providerData == nil || r.providerData.Client == nil {
		return
	}
	network := plan.NetworkID.ValueString()
	if _, err := r.providerData.Client.Collectors.Attach(ctx, network,
		forward.CollectorAttachmentRequest{Username: plan.Username.ValueString()}); err != nil {
		resp.Diagnostics.AddError("Unable to Attach Collector", err.Error())
		return
	}
	plan.ID = types.StringValue(network + "/" + plan.Username.ValueString())
	r.readInto(ctx, &plan, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *CollectorAttachmentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state collectorAttachmentResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() || r.providerData == nil || r.providerData.Client == nil {
		return
	}
	attachment, _, err := r.providerData.Client.Collectors.Attachment(ctx, state.NetworkID.ValueString())
	if err != nil {
		if isNotFoundError(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to Read Collector Attachment", err.Error())
		return
	}
	if !attachment.IsSet {
		resp.State.RemoveResource(ctx)
		return
	}
	state.Name = stringOrNull(attachment.CollectorName)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *CollectorAttachmentResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan collectorAttachmentResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() || r.providerData == nil || r.providerData.Client == nil {
		return
	}
	if _, err := r.providerData.Client.Collectors.Attach(ctx, plan.NetworkID.ValueString(),
		forward.CollectorAttachmentRequest{Username: plan.Username.ValueString()}); err != nil {
		resp.Diagnostics.AddError("Unable to Attach Collector", err.Error())
		return
	}
	plan.ID = types.StringValue(plan.NetworkID.ValueString() + "/" + plan.Username.ValueString())
	r.readInto(ctx, &plan, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete drops it from state only. Forward has no detach route, and a network
// keeps whatever collector it was given.
func (r *CollectorAttachmentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state collectorAttachmentResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.AddWarning("Collector Not Detached",
		fmt.Sprintf("Forward has no route to detach a collector, so network %s keeps the one it was given. "+
			"It is removed from Terraform state only.", state.NetworkID.ValueString()))
}

func (r *CollectorAttachmentResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *CollectorAttachmentResource) readInto(ctx context.Context, plan *collectorAttachmentResourceModel, diags *diag.Diagnostics) {
	attachment, _, err := r.providerData.Client.Collectors.Attachment(ctx, plan.NetworkID.ValueString())
	if err != nil {
		diags.AddError("Unable to Read Collector Attachment", err.Error())
		return
	}
	plan.Name = stringOrNull(attachment.CollectorName)
}
