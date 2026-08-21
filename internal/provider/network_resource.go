// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &NetworkResource{}
	_ resource.ResourceWithImportState = &NetworkResource{}
)

// NetworkResource manages a Forward network, which is the container everything
// else in this provider hangs off: cloud accounts, snapshots, checks.
type NetworkResource struct {
	providerData *ForwardProviderData
}

type networkResourceModel struct {
	ID    types.String `tfsdk:"id"`
	Name  types.String `tfsdk:"name"`
	OrgID types.String `tfsdk:"org_id"`
}

func NewNetworkResource() resource.Resource { return &NetworkResource{} }

func (r *NetworkResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_network"
}

func (r *NetworkResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A Forward network. Everything else — cloud accounts, snapshots, checks — belongs to one.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Identifier Forward assigns the network.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required: true,
				// Forward has no rename route, so a new name is a new network.
				// Destroying one takes its snapshots with it.
				MarkdownDescription: "Network name. Changing it replaces the network, and its snapshots with it.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"org_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Organization the network belongs to.",
			},
		},
	}
}

func (r *NetworkResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *NetworkResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan networkResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() || !r.configured(&resp.Diagnostics) {
		return
	}
	network, _, err := r.providerData.Client.Networks.Create(ctx, plan.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Create Network", err.Error())
		return
	}
	plan.ID = types.StringValue(string(network.ID))
	plan.OrgID = stringOrNull(string(network.OrgID))
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *NetworkResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state networkResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() || r.providerData == nil || r.providerData.Client == nil {
		return
	}
	networks, _, err := r.providerData.Client.Networks.List(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Networks", err.Error())
		return
	}
	for _, network := range networks {
		if string(network.ID) == state.ID.ValueString() {
			state.Name = types.StringValue(network.Name)
			state.OrgID = stringOrNull(string(network.OrgID))
			resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
			return
		}
	}
	// Deleted outside Terraform.
	resp.State.RemoveResource(ctx)
}

// Update cannot be reached: the only settable attribute requires replacement.
func (r *NetworkResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan networkResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *NetworkResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state networkResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() || r.providerData == nil || r.providerData.Client == nil {
		return
	}
	if _, _, err := r.providerData.Client.Networks.Delete(ctx, state.ID.ValueString()); err != nil {
		if isNotFoundError(err) {
			return
		}
		resp.Diagnostics.AddError("Unable to Delete Network", err.Error())
	}
}

func (r *NetworkResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *NetworkResource) configured(diags *diag.Diagnostics) bool {
	if r.providerData == nil || r.providerData.Client == nil {
		diags.AddError("Unconfigured Provider", "The provider client was not configured.")
		return false
	}
	return true
}
