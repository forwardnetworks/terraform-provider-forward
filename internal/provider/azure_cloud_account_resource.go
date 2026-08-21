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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &AzureCloudAccountResource{}
	_ resource.ResourceWithImportState = &AzureCloudAccountResource{}
)

// AzureCloudAccountResource manages an Azure collection source.
//
// Separate from the AWS resource despite one endpoint serving both, because
// the two authenticate nothing alike: AWS has keys or an assumed role, Azure
// has a service principal against a tenant. One resource covering both would
// be a pile of fields that are required half the time.
type AzureCloudAccountResource struct {
	providerData *ForwardProviderData
}

type azureCloudAccountResourceModel struct {
	ID              types.String `tfsdk:"id"`
	NetworkID       types.String `tfsdk:"network_id"`
	Name            types.String `tfsdk:"name"`
	Collect         types.Bool   `tfsdk:"collect"`
	ClientID        types.String `tfsdk:"client_id"`
	ClientSecret    types.String `tfsdk:"client_secret"`
	Tenant          types.String `tfsdk:"tenant"`
	Environment     types.String `tfsdk:"environment"`
	SubscriptionIDs types.List   `tfsdk:"subscription_ids"`
	ProxyServerID   types.String `tfsdk:"proxy_server_id"`
	DeleteOnDestroy types.Bool   `tfsdk:"delete_on_destroy"`
}

func NewAzureCloudAccountResource() resource.Resource { return &AzureCloudAccountResource{} }

func (r *AzureCloudAccountResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_azure_cloud_account"
}

func (r *AzureCloudAccountResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "An Azure collection source: a service principal, a tenant, and the subscriptions to collect.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"network_id": schema.StringAttribute{
				Required:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Name of the setup. This is its identity for every other call.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"collect": schema.BoolAttribute{
				Optional: true, Computed: true, Default: booldefault.StaticBool(true),
				MarkdownDescription: "Whether Forward collects from this source.",
			},
			"client_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Application (client) ID of the service principal.",
			},
			"client_secret": schema.StringAttribute{
				Optional: true, Sensitive: true,
				MarkdownDescription: "Client secret. Forward does not read it back, so Terraform cannot detect " +
					"a change made elsewhere.",
			},
			"tenant": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Directory (tenant) ID.",
			},
			"environment": schema.StringAttribute{
				Optional: true, Computed: true, Default: stringdefault.StaticString("AZURE"),
				MarkdownDescription: "Azure cloud: `AZURE`, `AZURE_US_GOVERNMENT`, `AZURE_CHINA`.",
			},
			"subscription_ids": schema.ListAttribute{
				Required:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Subscriptions to collect. Forward collects once per subscription.",
			},
			"proxy_server_id": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Proxy collection goes through.",
			},
			"delete_on_destroy": schema.BoolAttribute{
				Optional: true, Computed: true, Default: booldefault.StaticBool(true),
				MarkdownDescription: "Remove the setup from Forward on destroy. Off adopts an existing setup " +
					"without taking responsibility for removing it.",
			},
		},
	}
}

func (r *AzureCloudAccountResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *AzureCloudAccountResource) request(ctx context.Context, plan azureCloudAccountResourceModel,
	diags *diag.Diagnostics) forward.CloudAccountRequest {
	var subscriptions []string
	diags.Append(plan.SubscriptionIDs.ElementsAs(ctx, &subscriptions, false)...)

	request := forward.CloudAccountRequest{
		Type:     "AZURE",
		Name:     plan.Name.ValueString(),
		Collect:  boolPointer(plan.Collect),
		ClientID: plan.ClientID.ValueString(),
		// Forward carries the secret in "password" whatever the provider is,
		// and requires it. clientSecret alone is refused outright.
		Password:        plan.ClientSecret.ValueString(),
		Tenant:          plan.Tenant.ValueString(),
		Environment:     plan.Environment.ValueString(),
		SubscriptionIDs: subscriptions,
	}
	// Forward requires a last-test timestamp per subscription on a create, and
	// refuses the request without one. Zero means "never tested", which is
	// true of a setup that has not run yet.
	if len(subscriptions) != 0 {
		request.TestInstants = make(map[string]int64, len(subscriptions))
		for _, subscription := range subscriptions {
			request.TestInstants[subscription] = 0
		}
	}
	if !plan.ProxyServerID.IsNull() && !plan.ProxyServerID.IsUnknown() {
		proxy := plan.ProxyServerID.ValueString()
		request.ProxyServerID = &proxy
	}
	return request
}

func (r *AzureCloudAccountResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan azureCloudAccountResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() || r.providerData == nil || r.providerData.Client == nil {
		return
	}
	network := plan.NetworkID.ValueString()
	request := r.request(ctx, plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	if _, _, err := r.providerData.Client.CloudAccounts.Create(ctx, network, request); err != nil {
		resp.Diagnostics.AddError("Unable to Create Azure Cloud Account", err.Error())
		return
	}
	plan.ID = types.StringValue(network + "/" + plan.Name.ValueString())
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *AzureCloudAccountResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state azureCloudAccountResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() || r.providerData == nil || r.providerData.Client == nil {
		return
	}
	account, _, err := r.providerData.Client.CloudAccounts.Get(ctx,
		state.NetworkID.ValueString(), state.Name.ValueString())
	if err != nil {
		if isNotFoundError(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to Read Azure Cloud Account", err.Error())
		return
	}
	state.Collect = types.BoolValue(account.Collect)
	state.ProxyServerID = stringOrNull(account.ProxyServerID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *AzureCloudAccountResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan azureCloudAccountResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() || r.providerData == nil || r.providerData.Client == nil {
		return
	}
	request := r.request(ctx, plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	if _, _, err := r.providerData.Client.CloudAccounts.Update(ctx,
		plan.NetworkID.ValueString(), plan.Name.ValueString(), request); err != nil {
		resp.Diagnostics.AddError("Unable to Update Azure Cloud Account", err.Error())
		return
	}
	plan.ID = types.StringValue(plan.NetworkID.ValueString() + "/" + plan.Name.ValueString())
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *AzureCloudAccountResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state azureCloudAccountResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() || r.providerData == nil || r.providerData.Client == nil {
		return
	}
	if !state.DeleteOnDestroy.ValueBool() {
		return
	}
	if _, err := r.providerData.Client.CloudAccounts.Delete(ctx,
		state.NetworkID.ValueString(), state.Name.ValueString()); err != nil && !isNotFoundError(err) {
		resp.Diagnostics.AddError("Unable to Delete Azure Cloud Account", err.Error())
	}
}

func (r *AzureCloudAccountResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
