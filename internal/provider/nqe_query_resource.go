// Copyright (c) HashiCorp, Inc.

package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/forwardnetworks/terraform-provider-forward/internal/sdk"
)

var _ resource.Resource = &NQEQueryResource{}
var _ resource.ResourceWithImportState = &NQEQueryResource{}

// NQEQueryResource models a Forward NQE library entry reference.
type NQEQueryResource struct {
	providerData *ForwardProviderData
}

// NQEQueryResourceModel maps Terraform schema data.
type NQEQueryResourceModel struct {
	ID         types.String `tfsdk:"id"`
	Path       types.String `tfsdk:"path"`
	Repository types.String `tfsdk:"repository"`
	Intent     types.String `tfsdk:"intent"`
	QueryID    types.String `tfsdk:"query_id"`
}

func NewNQEQueryResource() resource.Resource {
	return &NQEQueryResource{}
}

func (r *NQEQueryResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_nqe_query_definition"
}

func (r *NQEQueryResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reference a Forward Enterprise NQE library entry by path and repository.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Internal Terraform identifier (mirrors query_id).",
			},
			"path": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Unique NQE library path (for example, /L3/MtuConsistency).",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"repository": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Source repository for the query (e.g. ORG or FWD).",
				Default:             stringdefault.StaticString("ORG"),
			},
			"intent": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Intent string associated with the query.",
			},
			"query_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Forward Enterprise query identifier.",
			},
		},
	}
}

func (r *NQEQueryResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *NQEQueryResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	if r.providerData == nil {
		resp.Diagnostics.AddError(
			"Unconfigured Provider",
			"The provider client was not configured. Re-run terraform init or review provider configuration.",
		)
		return
	}

	var plan NQEQueryResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	query, diags := r.lookupQuery(ctx, plan.Path.ValueString(), plan.Repository.ValueString())
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if query == nil {
		resp.Diagnostics.AddError(
			"NQE query not found",
			"The specified NQE query does not exist in the Forward library. New query creation is not currently supported via API; create the query in Forward Enterprise and re-run Terraform.",
		)
		return
	}

	plan.QueryID = types.StringValue(query.QueryID)
	plan.Intent = stringOrNull(query.Intent)
	plan.Repository = types.StringValue(query.Repository)
	plan.ID = plan.QueryID

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *NQEQueryResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	if r.providerData == nil {
		resp.Diagnostics.AddError(
			"Unconfigured Provider",
			"The provider client was not configured. Re-run terraform init or review provider configuration.",
		)
		return
	}

	var state NQEQueryResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	query, diags := r.lookupQuery(ctx, state.Path.ValueString(), state.Repository.ValueString())
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if query == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	state.QueryID = types.StringValue(query.QueryID)
	state.Intent = stringOrNull(query.Intent)
	state.Repository = types.StringValue(query.Repository)
	state.ID = state.QueryID

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *NQEQueryResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// All fields require replacement; nothing to do.
	var plan NQEQueryResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *NQEQueryResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	// Query deletion is not performed via API; removing from state is sufficient.
}

func (r *NQEQueryResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("query_id"), req, resp)
}

func (r *NQEQueryResource) lookupQuery(ctx context.Context, queryPath, repository string) (*sdk.NqeQuery, diag.Diagnostics) {
	var diags diag.Diagnostics

	if strings.TrimSpace(queryPath) == "" {
		diags.AddAttributeError(path.Root("path"), "Missing Path", "path must be provided.")
		return nil, diags
	}

	queries, err := r.providerData.Client.ListNQEQueries(ctx, "")
	if err != nil {
		diags.AddError("Error listing NQE queries", err.Error())
		return nil, diags
	}

	for _, q := range queries {
		if q.Path == queryPath && strings.EqualFold(q.Repository, repository) {
			return &q, diags
		}
	}

	return nil, diags
}
