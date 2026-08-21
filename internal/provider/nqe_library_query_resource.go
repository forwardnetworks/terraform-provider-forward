// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"strings"

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
	_ resource.Resource                = &NQELibraryQueryResource{}
	_ resource.ResourceWithImportState = &NQELibraryQueryResource{}
)

// NQELibraryQueryResource publishes a query into the organization's NQE
// library.
//
// Distinct from forward_nqe_query_definition, which only looks up a query
// somebody else already wrote. This one owns the query's content.
type NQELibraryQueryResource struct {
	providerData *ForwardProviderData
}

type nqeLibraryQueryResourceModel struct {
	ID          types.String `tfsdk:"id"`
	QueryPath   types.String `tfsdk:"path"`
	SourceCode  types.String `tfsdk:"source_code"`
	CommitTitle types.String `tfsdk:"commit_title"`
	QueryID     types.String `tfsdk:"query_id"`
	Intent      types.String `tfsdk:"intent"`
}

func NewNQELibraryQueryResource() resource.Resource {
	return &NQELibraryQueryResource{}
}

func (r *NQELibraryQueryResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_nqe_library_query"
}

func (r *NQELibraryQueryResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Publish a query to the organization's NQE library. The query becomes visible in the " +
			"library and can back an NQE intent check.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Mirrors `path`, which is the query's identity in the library.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"path": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "Library path, for example `/Demo/Security/Open Security Groups`. " +
					"Parent directories are created as needed. Changing it replaces the query.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"source_code": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "NQE source. Updating it publishes a new commit.",
			},
			"commit_title": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Title recorded on the library commit.",
			},
			"query_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Identifier Forward assigns the published query, for use in an NQE check.",
			},
			"intent": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Intent string Forward derives from the query.",
			},
		},
	}
}

func (r *NQELibraryQueryResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *NQELibraryQueryResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan nqeLibraryQueryResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() || !r.configured(&resp.Diagnostics) {
		return
	}
	r.publish(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *NQELibraryQueryResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state nqeLibraryQueryResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() || r.providerData == nil || r.providerData.Client == nil {
		return
	}
	queryPath := state.QueryPath.ValueString()
	published, err := r.find(ctx, queryPath)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read NQE Library Query", err.Error())
		return
	}
	if published == nil {
		// Removed from the library outside Terraform.
		resp.State.RemoveResource(ctx)
		return
	}
	state.QueryID = types.StringValue(published.QueryID)
	state.Intent = stringOrNull(published.Intent)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *NQELibraryQueryResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan nqeLibraryQueryResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() || !r.configured(&resp.Diagnostics) {
		return
	}
	r.publish(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *NQELibraryQueryResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state nqeLibraryQueryResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() || r.providerData == nil || r.providerData.Client == nil {
		return
	}
	queryPath := state.QueryPath.ValueString()
	client := r.providerData.Client
	if _, err := client.NQERepository.DeleteQuery(ctx, queryPath); err != nil {
		resp.Diagnostics.AddError("Unable to Delete NQE Library Query", err.Error())
		return
	}
	// The draft deletion is invisible to everyone else until it is committed,
	// so a failure here leaves the query still published.
	if _, err := client.NQERepository.Commit(ctx, forward.NQECommitRequest{
		Paths:   []string{queryPath},
		Message: &forward.NQECommitMessage{Title: "terraform: remove " + queryPath},
	}); err != nil {
		resp.Diagnostics.AddError("Unable to Commit NQE Library Removal", err.Error())
	}
}

func (r *NQELibraryQueryResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("path"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

// publish writes the draft and commits it. Both steps are required: a draft
// lives in the calling user's workspace and is invisible to the library, to
// colleagues, and to any check that would reference it.
func (r *NQELibraryQueryResource) publish(ctx context.Context, plan *nqeLibraryQueryResourceModel, diags *diag.Diagnostics) {
	client := r.providerData.Client
	queryPath := strings.TrimSpace(plan.QueryPath.ValueString())

	// Every level, outermost first: Forward creates one directory at a time
	// and refuses a query whose immediate parent does not exist, so asking
	// only for the immediate parent fails on any nested path.
	for _, dir := range ancestorDirectories(queryPath) {
		if _, err := client.NQERepository.AddDirectory(ctx, dir); err != nil {
			diags.AddError("Unable to Create NQE Library Directory", err.Error())
			return
		}
	}
	if _, err := client.NQERepository.AddQuery(ctx, queryPath, plan.SourceCode.ValueString()); err != nil {
		diags.AddError("Unable to Save NQE Library Query", err.Error())
		return
	}

	title := plan.CommitTitle.ValueString()
	if plan.CommitTitle.IsNull() || plan.CommitTitle.IsUnknown() || title == "" {
		title = "terraform: publish " + queryPath
	}
	plan.CommitTitle = types.StringValue(title)
	if _, err := client.NQERepository.Commit(ctx, forward.NQECommitRequest{
		Paths:   []string{queryPath},
		Message: &forward.NQECommitMessage{Title: title},
	}); err != nil {
		diags.AddError("Unable to Commit NQE Library Query", err.Error())
		return
	}

	published, err := r.find(ctx, queryPath)
	if err != nil {
		diags.AddError("Unable to Read Published NQE Query", err.Error())
		return
	}
	if published == nil {
		diags.AddError("NQE Query Not Published",
			fmt.Sprintf("Forward accepted the commit but %s is not in the library.", queryPath))
		return
	}
	plan.ID = types.StringValue(queryPath)
	plan.QueryID = types.StringValue(published.QueryID)
	plan.Intent = stringOrNull(published.Intent)
}

// find locates the published query. The listing is scoped to the query's own
// directory, so a large library costs no more than a small one.
func (r *NQELibraryQueryResource) find(ctx context.Context, queryPath string) (*forward.NQEQuery, error) {
	queries, _, err := r.providerData.Client.NQE.ListQueries(ctx, parentDirectory(queryPath))
	if err != nil {
		return nil, err
	}
	for i := range queries {
		if queries[i].Path == queryPath && strings.EqualFold(queries[i].Repository, "ORG") {
			return &queries[i], nil
		}
	}
	return nil, nil
}

func (r *NQELibraryQueryResource) configured(diags *diag.Diagnostics) bool {
	if r.providerData == nil || r.providerData.Client == nil {
		diags.AddError("Unconfigured Provider", "The provider client was not configured.")
		return false
	}
	return true
}

// parentDirectory returns the containing directory, with the trailing slash
// Forward requires. "/A/B/C" -> "/A/B/".
func parentDirectory(queryPath string) string {
	index := strings.LastIndex(queryPath, "/")
	if index < 0 {
		return ""
	}
	return queryPath[:index+1]
}

// ancestorDirectories lists every directory containing queryPath, outermost
// first. "/A/B/C" -> ["/A/", "/A/B/"].
func ancestorDirectories(queryPath string) []string {
	parent := parentDirectory(queryPath)
	if parent == "" || parent == "/" {
		return nil
	}
	var dirs []string
	for index, char := range parent {
		if char == '/' && index > 0 {
			dirs = append(dirs, parent[:index+1])
		}
	}
	return dirs
}
