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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &ProxyResource{}
	_ resource.ResourceWithImportState = &ProxyResource{}
)

// ProxyResource manages a proxy server a network's collection goes through.
//
// This is what makes collection reach somewhere other than the real internet:
// point it at a local endpoint and Forward collects from that instead.
type ProxyResource struct {
	providerData *ForwardProviderData
}

type proxyResourceModel struct {
	ID                  types.String `tfsdk:"id"`
	NetworkID           types.String `tfsdk:"network_id"`
	Name                types.String `tfsdk:"name"`
	Host                types.String `tfsdk:"host"`
	Port                types.Int64  `tfsdk:"port"`
	Protocol            types.String `tfsdk:"protocol"`
	DisableCertChecking types.Bool   `tfsdk:"disable_cert_checking"`
}

func NewProxyResource() resource.Resource { return &ProxyResource{} }

func (r *ProxyResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_proxy"
}

func (r *ProxyResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A proxy server collection goes through. Point it at a local endpoint and Forward " +
			"collects from that rather than from the real provider.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Identifier Forward assigns the proxy.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"network_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Network the proxy belongs to.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"name":     schema.StringAttribute{Required: true, MarkdownDescription: "Name shown in Forward."},
			"host":     schema.StringAttribute{Required: true, MarkdownDescription: "Host the proxy listens on."},
			"port":     schema.Int64Attribute{Required: true, MarkdownDescription: "Port the proxy listens on."},
			"protocol": schema.StringAttribute{Required: true, MarkdownDescription: "`HTTP` or `HTTPS`."},
			"disable_cert_checking": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(false),
				MarkdownDescription: "Skip certificate verification through this proxy. Needed when the far side " +
					"presents a certificate the appserver has no reason to trust.",
			},
		},
	}
}

func (r *ProxyResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *ProxyResource) input(plan proxyResourceModel) forward.ProxyServer {
	return forward.ProxyServer{
		Name:                plan.Name.ValueString(),
		Host:                plan.Host.ValueString(),
		Port:                int(plan.Port.ValueInt64()),
		Protocol:            plan.Protocol.ValueString(),
		DisableCertChecking: plan.DisableCertChecking.ValueBool(),
	}
}

func (r *ProxyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan proxyResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() || !r.configured(&resp.Diagnostics) {
		return
	}
	proxy, _, err := r.providerData.Client.Proxies.Create(ctx, plan.NetworkID.ValueString(), r.input(plan))
	if err != nil {
		resp.Diagnostics.AddError("Unable to Create Proxy", err.Error())
		return
	}
	plan.ID = types.StringValue(string(proxy.ID))
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ProxyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state proxyResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() || r.providerData == nil || r.providerData.Client == nil {
		return
	}
	proxies, _, err := r.providerData.Client.Proxies.List(ctx, state.NetworkID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Proxies", err.Error())
		return
	}
	for _, proxy := range proxies {
		if string(proxy.ID) == state.ID.ValueString() {
			state.Name = types.StringValue(proxy.Name)
			state.Host = types.StringValue(proxy.Host)
			state.Port = types.Int64Value(int64(proxy.Port))
			state.Protocol = types.StringValue(proxy.Protocol)
			state.DisableCertChecking = types.BoolValue(proxy.DisableCertChecking)
			resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
			return
		}
	}
	resp.State.RemoveResource(ctx)
}

func (r *ProxyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan proxyResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	var state proxyResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() || !r.configured(&resp.Diagnostics) {
		return
	}
	plan.ID = state.ID
	if _, _, err := r.providerData.Client.Proxies.Update(ctx,
		plan.NetworkID.ValueString(), plan.ID.ValueString(), r.input(plan)); err != nil {
		resp.Diagnostics.AddError("Unable to Update Proxy", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete is a no-op with a warning: Forward exposes no route to remove a proxy,
// so reporting success would claim something that did not happen.
func (r *ProxyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state proxyResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.AddWarning("Proxy Not Deleted",
		fmt.Sprintf("Forward has no route to delete a proxy server, so %s remains configured on network %s. "+
			"It is removed from Terraform state only.",
			state.Name.ValueString(), state.NetworkID.ValueString()))
}

func (r *ProxyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *ProxyResource) configured(diags *diag.Diagnostics) bool {
	if r.providerData == nil || r.providerData.Client == nil {
		diags.AddError("Unconfigured Provider", "The provider client was not configured.")
		return false
	}
	return true
}
