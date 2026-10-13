// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	forward "github.com/forwardnetworks/forward-go-sdk"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &CollectorResource{}
	_ resource.ResourceWithImportState = &CollectorResource{}
)

// CollectorResource registers a collector with Forward and keeps its token.
//
// Forward returns the token once, in the registration response; there is no
// route that reads it back. So the token lives only in state, Read never
// touches it, and an imported collector has none.
type CollectorResource struct {
	providerData *ForwardProviderData
}

type collectorResourceModel struct {
	ID       types.String `tfsdk:"id"`
	Name     types.String `tfsdk:"name"`
	Username types.String `tfsdk:"username"`
	Token    types.String `tfsdk:"token"`
	AppURL   types.String `tfsdk:"app_server_url"`
}

func NewCollectorResource() resource.Resource { return &CollectorResource{} }

func (r *CollectorResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_collector"
}

func (r *CollectorResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Registers a collector with Forward and returns the token it runs with. " +
			"Forward shows the token only at registration, so it is kept in state and cannot be recovered " +
			"after an import. Destroying this deletes the collector in Forward.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Forward's collector ID.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Collector name, unique in the organization. Changing it registers a new collector.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"username": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The collector's username (`collector-xxxxxxxx`), as `forward_collector_attachment` takes it.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"token": schema.StringAttribute{
				Computed:  true,
				Sensitive: true,
				MarkdownDescription: "The collector's auth token as `username:password`: the `TOKEN` the collector " +
					"container takes. Null for an imported collector.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"app_server_url": schema.StringAttribute{
				Computed: true,
				MarkdownDescription: "Forward server the token was issued for, when Forward includes it in the " +
					"token (newer token format); null otherwise.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *CollectorResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *CollectorResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan collectorResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() || r.providerData == nil || r.providerData.Client == nil {
		return
	}
	name := plan.Name.ValueString()
	// A collector registered without a usable token is deleted again rather
	// than left as an orphan whose token nobody can ever see.
	discard := func() { _, _ = r.providerData.Client.Collectors.Delete(ctx, name) }
	reg, _, err := r.providerData.Client.Collectors.Register(ctx,
		forward.CollectorRegistrationRequest{CollectorName: name})
	if err != nil {
		if reg != nil && reg.ID != "" {
			discard()
		}
		resp.Diagnostics.AddError("Unable to Register Collector", err.Error())
		return
	}
	key, err := reg.Key()
	if err != nil {
		discard()
		resp.Diagnostics.AddError("Unable to Read Collector Token", err.Error())
		return
	}
	plan.ID = types.StringValue(string(reg.ID))
	if reg.Name != "" {
		plan.Name = types.StringValue(reg.Name)
	}
	plan.Username = types.StringValue(key.Username)
	plan.Token = types.StringValue(key.Token())
	plan.AppURL = stringOrNull(key.AppURL)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *CollectorResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state collectorResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() || r.providerData == nil || r.providerData.Client == nil {
		return
	}
	// By ID only: a collector re-registered under the same name is a different
	// collector with a different token, and must be re-created, not adopted.
	collector, err := r.find(ctx, state.ID.ValueString(), "")
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Collector", err.Error())
		return
	}
	if collector == nil {
		resp.State.RemoveResource(ctx)
		return
	}
	state.ID = types.StringValue(string(collector.ID))
	state.Name = types.StringValue(collector.Name)
	state.Username = types.StringValue(collector.Username)
	if state.Token.IsUnknown() {
		state.Token = types.StringNull()
	}
	if state.AppURL.IsUnknown() {
		state.AppURL = types.StringNull()
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update has nothing to send: name forces replacement and the rest is computed.
func (r *CollectorResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan collectorResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *CollectorResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state collectorResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() || r.providerData == nil || r.providerData.Client == nil {
		return
	}
	if _, err := r.providerData.Client.Collectors.Delete(ctx, state.Name.ValueString()); err != nil && !isNotFoundError(err) {
		resp.Diagnostics.AddError("Unable to Delete Collector", err.Error())
	}
}

// ImportState takes the collector's name or ID. The token cannot be imported.
func (r *CollectorResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	collector, err := r.find(ctx, req.ID, req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Import Collector", err.Error())
		return
	}
	if collector == nil {
		resp.Diagnostics.AddError("Collector Not Found", fmt.Sprintf("No collector has the name or ID %q.", req.ID))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), string(collector.ID))...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), collector.Name)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("username"), collector.Username)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("token"), types.StringNull())...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("app_server_url"), types.StringNull())...)
	resp.Diagnostics.AddWarning("Collector Token Not Imported",
		"Forward shows a collector's token only when it is registered, so token is null after import.")
}

// find looks the collector up by ID first, then by name; nil if neither matches.
func (r *CollectorResource) find(ctx context.Context, id, name string) (*forward.Collector, error) {
	list, _, err := r.providerData.Client.Collectors.List(ctx)
	if err != nil {
		return nil, err
	}
	for i := range list {
		if id != "" && string(list[i].ID) == id {
			return &list[i], nil
		}
	}
	for i := range list {
		if name != "" && list[i].Name == name {
			return &list[i], nil
		}
	}
	return nil, nil
}
