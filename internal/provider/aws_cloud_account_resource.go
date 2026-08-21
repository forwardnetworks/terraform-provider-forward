// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	schemavalidator "github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

	forward "github.com/forwardnetworks/forward-go-sdk"
)

const (
	awsCredentialModeForwardAssumeRole = "forward-assume-role"
	awsCredentialModeStaticKeys        = "static-keys"
	awsCredentialModeInstanceProfile   = "instance-profile"
	redactedSecretValue                = "<redacted>"
)

var _ resource.Resource = &AWSCloudAccountResource{}
var _ resource.ResourceWithImportState = &AWSCloudAccountResource{}

// NewAWSCloudAccountResource instantiates the AWS cloud account resource.
func NewAWSCloudAccountResource() resource.Resource {
	return &AWSCloudAccountResource{}
}

// AWSCloudAccountResource manages a Forward AWS cloud account setup.
type AWSCloudAccountResource struct {
	providerData *ForwardProviderData
}

type awsCloudAccountResourceModel struct {
	ID                            types.String             `tfsdk:"id"`
	NetworkID                     types.String             `tfsdk:"network_id"`
	Name                          types.String             `tfsdk:"name"`
	Collect                       types.Bool               `tfsdk:"collect"`
	Regions                       types.Set                `tfsdk:"regions"`
	ProxyServerID                 types.String             `tfsdk:"proxy_server_id"`
	RegionToProxyServerID         types.Map                `tfsdk:"region_to_proxy_server_id"`
	AssumeRoleInfos               []awsAssumeRoleInfoModel `tfsdk:"assume_role_infos"`
	CredentialMode                types.String             `tfsdk:"credential_mode"`
	CollectorAccessKeyID          types.String             `tfsdk:"collector_access_key_id"`
	CollectorSecretAccessKey      types.String             `tfsdk:"collector_secret_access_key"`
	UseForwardAccountToAssumeRole types.Bool               `tfsdk:"use_forward_account_to_assume_role"`
	Concurrency                   types.Int64              `tfsdk:"concurrency"`
	ConnectionTimeoutSeconds      types.Int64              `tfsdk:"connection_timeout_seconds"`
	RequestTimeoutSeconds         types.Int64              `tfsdk:"request_timeout_seconds"`
	AllowAccountRemovals          types.Bool               `tfsdk:"allow_account_removals"`
	DeleteOnDestroy               types.Bool               `tfsdk:"delete_on_destroy"`

	AccountCount          types.Int64  `tfsdk:"account_count"`
	PayloadJSON           types.String `tfsdk:"payload_json"`
	ManualAccountDataJSON types.String `tfsdk:"manual_account_data_json"`
}

func (r *AWSCloudAccountResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_aws_cloud_account"
}

func (r *AWSCloudAccountResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manage a Forward AWS cloud account setup. Pair this resource with the forward_aws_organization_accounts data source to keep account membership synchronized from AWS Organizations.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Terraform resource ID in network_id/name form.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"network_id": schema.StringAttribute{
				MarkdownDescription: "Forward network ID. Defaults to the provider network_id when omitted.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Forward AWS cloud setup name.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"collect": schema.BoolAttribute{
				MarkdownDescription: "Whether Forward should collect this AWS setup.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
			},
			"regions": schema.SetAttribute{
				MarkdownDescription: "AWS regions Forward should collect.",
				ElementType:         types.StringType,
				Required:            true,
			},
			"proxy_server_id": schema.StringAttribute{
				MarkdownDescription: "Optional default Forward proxy server ID for this setup.",
				Optional:            true,
			},
			"region_to_proxy_server_id": schema.MapAttribute{
				MarkdownDescription: "Optional map from AWS region to Forward proxy server ID.",
				ElementType:         types.StringType,
				Optional:            true,
			},
			"assume_role_infos": schema.ListNestedAttribute{
				MarkdownDescription: "AWS accounts and role ARNs Forward should collect. Usually set from data.forward_aws_organization_accounts.*.assume_role_infos.",
				Required:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: awsAssumeRoleInfoResourceAttributes(),
				},
			},
			"credential_mode": schema.StringAttribute{
				MarkdownDescription: fmt.Sprintf(
					"AWS collection credential model. `%s` uses Forward's AWS account and the Forward-generated external ID. `%s` stores an AWS access key ID and secret access key in Forward and uses that key to assume member-account roles. `%s` sends no stored AWS keys and lets the collector AWS SDK credential chain, usually an EC2 instance profile, assume member-account roles.",
					awsCredentialModeForwardAssumeRole,
					awsCredentialModeStaticKeys,
					awsCredentialModeInstanceProfile,
				),
				Optional: true,
				Computed: true,
				Validators: []schemavalidator.String{
					stringvalidator.OneOf(
						awsCredentialModeForwardAssumeRole,
						awsCredentialModeStaticKeys,
						awsCredentialModeInstanceProfile,
					),
				},
			},
			"collector_access_key_id": schema.StringAttribute{
				MarkdownDescription: "AWS access key ID stored in Forward when credential_mode is static-keys. Use a Terraform variable sourced from runtime secret storage.",
				Optional:            true,
				Sensitive:           true,
			},
			"collector_secret_access_key": schema.StringAttribute{
				MarkdownDescription: "AWS secret access key stored in Forward when credential_mode is static-keys. This value is sensitive and will be sent only to Forward's create or credential update APIs.",
				Optional:            true,
				Sensitive:           true,
			},
			"use_forward_account_to_assume_role": schema.BoolAttribute{
				MarkdownDescription: "Rendered Forward API mode flag. Prefer credential_mode for configuration.",
				Optional:            true,
				Computed:            true,
				DeprecationMessage:  "Use credential_mode instead. This field remains for compatibility and API visibility.",
			},
			"concurrency": schema.Int64Attribute{
				MarkdownDescription: "Optional per-cloud-account AWS API concurrency setting.",
				Optional:            true,
			},
			"connection_timeout_seconds": schema.Int64Attribute{
				MarkdownDescription: "Optional AWS API connection timeout in seconds.",
				Optional:            true,
			},
			"request_timeout_seconds": schema.Int64Attribute{
				MarkdownDescription: "Optional AWS API request timeout in seconds.",
				Optional:            true,
			},
			"allow_account_removals": schema.BoolAttribute{
				MarkdownDescription: "Allow Terraform to remove AWS account entries from an existing Forward setup. Defaults to false so a partial Organizations read cannot shrink collection without explicit confirmation.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"delete_on_destroy": schema.BoolAttribute{
				MarkdownDescription: "Delete the Forward cloud account setup when this Terraform resource is destroyed. Defaults to false; destroy otherwise only removes Terraform state.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"account_count": schema.Int64Attribute{
				MarkdownDescription: "Number of assume-role accounts in the current Forward setup.",
				Computed:            true,
			},
			"payload_json": schema.StringAttribute{
				MarkdownDescription: "Rendered Forward AWS cloud account API payload for review or manual POST/PATCH testing.",
				Computed:            true,
			},
			"manual_account_data_json": schema.StringAttribute{
				MarkdownDescription: "JSON array compatible with Forward's manual AWS account upload flow.",
				Computed:            true,
			},
		},
	}
}

func (r *AWSCloudAccountResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *AWSCloudAccountResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	if r.providerData == nil {
		resp.Diagnostics.AddError("Unconfigured Provider", "The provider client was not configured. Re-run terraform init or review provider configuration.")
		return
	}

	var plan awsCloudAccountResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	networkID := r.networkID(plan.NetworkID)
	plan.NetworkID = types.StringValue(networkID)
	mode, modeDiags := resolveAWSCredentialMode(plan)
	resp.Diagnostics.Append(modeDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.CredentialMode = types.StringValue(mode)
	plan.UseForwardAccountToAssumeRole = types.BoolValue(mode == awsCredentialModeForwardAssumeRole)
	body, diags := buildAWSCloudAccountRequest(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	existing, _, err := r.providerData.Client.CloudAccounts.Get(ctx, networkID, plan.Name.ValueString())
	if err != nil && !isNotFoundError(err) {
		resp.Diagnostics.AddError("Error checking Forward AWS cloud account", err.Error())
		return
	}
	if existing != nil && !strings.EqualFold(existing.Type, "AWS") {
		resp.Diagnostics.AddError("Existing Cloud Account Is Not AWS", fmt.Sprintf("Forward cloud account %q already exists with type %q.", existing.Name, existing.Type))
		return
	}
	if existing != nil {
		if err := validateExistingAWSCredentialMode(existing, mode); err != nil {
			resp.Diagnostics.AddError("Existing AWS Credential Mode Differs", err.Error())
			return
		}
		resp.Diagnostics.Append(accountRemovalDiagnostics(plan, existing, body.AssumeRoleInfos)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if _, _, err := r.providerData.Client.CloudAccounts.Update(ctx, networkID, plan.Name.ValueString(), awsCloudAccountPatchRequest(body)); err != nil {
			resp.Diagnostics.AddError("Error updating existing Forward AWS cloud account", err.Error())
			return
		}
		if mode == awsCredentialModeStaticKeys {
			if _, err := r.providerData.Client.CloudAccounts.UpdateCredential(ctx, networkID, plan.Name.ValueString(), awsCloudAccountCredentialRequest(plan)); err != nil {
				resp.Diagnostics.AddError("Error updating existing Forward AWS cloud account credentials", err.Error())
				return
			}
		}
	} else if _, _, err := r.providerData.Client.CloudAccounts.Create(ctx, networkID, body); err != nil {
		resp.Diagnostics.AddError("Error creating Forward AWS cloud account", err.Error())
		return
	}

	readDiags, err := r.readIntoState(ctx, &plan)
	resp.Diagnostics.Append(readDiags...)
	if err != nil {
		resp.Diagnostics.AddError("Error reading Forward AWS cloud account", err.Error())
		return
	}
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *AWSCloudAccountResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	if r.providerData == nil {
		resp.Diagnostics.AddError("Unconfigured Provider", "The provider client was not configured. Re-run terraform init or review provider configuration.")
		return
	}

	var state awsCloudAccountResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	readDiags, err := r.readIntoState(ctx, &state)
	resp.Diagnostics.Append(readDiags...)
	if err != nil {
		if isNotFoundError(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading Forward AWS cloud account", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *AWSCloudAccountResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	if r.providerData == nil {
		resp.Diagnostics.AddError("Unconfigured Provider", "The provider client was not configured. Re-run terraform init or review provider configuration.")
		return
	}

	var plan awsCloudAccountResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state awsCloudAccountResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	networkID := r.networkID(plan.NetworkID)
	plan.NetworkID = types.StringValue(networkID)
	mode, modeDiags := resolveAWSCredentialMode(plan)
	resp.Diagnostics.Append(modeDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	priorMode, priorModeDiags := resolveAWSCredentialMode(state)
	resp.Diagnostics.Append(priorModeDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if priorMode != "" && mode != priorMode {
		resp.Diagnostics.AddAttributeError(
			path.Root("credential_mode"),
			"Changing AWS Credential Mode Is Not Supported In Place",
			fmt.Sprintf("Forward does not accept credential mode changes through the cloud account PATCH endpoint. Current mode is %q and planned mode is %q. Create a new Forward AWS setup name, or change the mode manually in Forward and import/reconcile it.", priorMode, mode),
		)
		return
	}
	plan.CredentialMode = types.StringValue(mode)
	plan.UseForwardAccountToAssumeRole = types.BoolValue(mode == awsCredentialModeForwardAssumeRole)
	body, diags := buildAWSCloudAccountRequest(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	existing, _, err := r.providerData.Client.CloudAccounts.Get(ctx, networkID, plan.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error checking Forward AWS cloud account", err.Error())
		return
	}
	if !strings.EqualFold(existing.Type, "AWS") {
		resp.Diagnostics.AddError("Existing Cloud Account Is Not AWS", fmt.Sprintf("Forward cloud account %q already exists with type %q.", existing.Name, existing.Type))
		return
	}
	if err := validateExistingAWSCredentialMode(existing, mode); err != nil {
		resp.Diagnostics.AddError("Existing AWS Credential Mode Differs", err.Error())
		return
	}
	resp.Diagnostics.Append(accountRemovalDiagnostics(plan, existing, body.AssumeRoleInfos)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if _, _, err := r.providerData.Client.CloudAccounts.Update(ctx, networkID, plan.Name.ValueString(), awsCloudAccountPatchRequest(body)); err != nil {
		resp.Diagnostics.AddError("Error updating Forward AWS cloud account", err.Error())
		return
	}
	if mode == awsCredentialModeStaticKeys {
		if _, err := r.providerData.Client.CloudAccounts.UpdateCredential(ctx, networkID, plan.Name.ValueString(), awsCloudAccountCredentialRequest(plan)); err != nil {
			resp.Diagnostics.AddError("Error updating Forward AWS cloud account credentials", err.Error())
			return
		}
	}

	readDiags, err := r.readIntoState(ctx, &plan)
	resp.Diagnostics.Append(readDiags...)
	if err != nil {
		resp.Diagnostics.AddError("Error reading Forward AWS cloud account", err.Error())
		return
	}
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *AWSCloudAccountResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	if r.providerData == nil {
		resp.Diagnostics.AddError("Unconfigured Provider", "The provider client was not configured. Re-run terraform init or review provider configuration.")
		return
	}

	var state awsCloudAccountResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if state.DeleteOnDestroy.IsNull() || !state.DeleteOnDestroy.ValueBool() {
		return
	}

	if _, err := r.providerData.Client.CloudAccounts.Delete(ctx, r.networkID(state.NetworkID), state.Name.ValueString()); err != nil && !isNotFoundError(err) {
		resp.Diagnostics.AddError("Error deleting Forward AWS cloud account", err.Error())
	}
}

func (r *AWSCloudAccountResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, "/")
	if len(parts) != 2 {
		resp.Diagnostics.AddError("Invalid import format", "Use: network_id/cloud_account_name")
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("network_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), parts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

func (r *AWSCloudAccountResource) networkID(value types.String) string {
	if !value.IsNull() && !value.IsUnknown() && strings.TrimSpace(value.ValueString()) != "" {
		return strings.TrimSpace(value.ValueString())
	}
	return r.providerData.NetworkID
}

func (r *AWSCloudAccountResource) readIntoState(ctx context.Context, state *awsCloudAccountResourceModel) (diag.Diagnostics, error) {
	var diags diag.Diagnostics
	networkID := r.networkID(state.NetworkID)
	if networkID == "" {
		diags.AddAttributeError(path.Root("network_id"), "Missing Network ID", "Network ID must be specified either on the provider or resource.")
		return diags, nil
	}

	account, _, err := r.providerData.Client.CloudAccounts.Get(ctx, networkID, state.Name.ValueString())
	if err != nil {
		return diags, err
	}
	if !strings.EqualFold(account.Type, "AWS") {
		return diags, fmt.Errorf("cloud account %q exists with type %q, not AWS", account.Name, account.Type)
	}

	state.ID = types.StringValue(fmt.Sprintf("%s/%s", networkID, account.Name))
	state.NetworkID = types.StringValue(networkID)
	state.Name = types.StringValue(account.Name)
	state.Collect = types.BoolValue(account.Collect)
	state.ProxyServerID = stringOrNull(account.ProxyServerID)
	state.Regions = setOfStrings(regionNames(account.Regions))
	state.RegionToProxyServerID = mapOfStrings(ctx, account.RegionToProxyServerID)
	state.AssumeRoleInfos = flattenAWSAssumeRoleInfos(account.AssumeRoleInfos)
	mode := credentialModeFromStateAndAPI(*state, account)
	state.CredentialMode = types.StringValue(mode)
	state.UseForwardAccountToAssumeRole = types.BoolValue(mode == awsCredentialModeForwardAssumeRole)
	state.Concurrency = int64PointerOrNull(account.Concurrency)
	state.ConnectionTimeoutSeconds = int64PointerOrNull(account.ConnectionTimeoutSeconds)
	state.RequestTimeoutSeconds = int64PointerOrNull(account.RequestTimeoutSeconds)
	state.AccountCount = types.Int64Value(int64(len(account.AssumeRoleInfos)))
	state.PayloadJSON = payloadJSONForState(ctx, *state)
	state.ManualAccountDataJSON = manualAccountJSONForState(state.AssumeRoleInfos)
	return diags, nil
}

func buildAWSCloudAccountRequest(ctx context.Context, model awsCloudAccountResourceModel) (forward.CloudAccountRequest, diag.Diagnostics) {
	var diags diag.Diagnostics

	regions := stringSet(model.Regions)
	if len(regions) == 0 {
		diags.AddAttributeError(path.Root("regions"), "Missing Regions", "At least one AWS region is required.")
		return forward.CloudAccountRequest{}, diags
	}

	assumeRoleInfos, infoDiags := expandAWSAssumeRoleInfos(model.AssumeRoleInfos)
	diags.Append(infoDiags...)
	if diags.HasError() {
		return forward.CloudAccountRequest{}, diags
	}
	if len(assumeRoleInfos) == 0 {
		diags.AddAttributeError(path.Root("assume_role_infos"), "Missing AWS Accounts", "At least one assume_role_infos entry is required.")
		return forward.CloudAccountRequest{}, diags
	}

	regionProxyIDs := map[string]string{}
	if !model.RegionToProxyServerID.IsNull() && !model.RegionToProxyServerID.IsUnknown() {
		mapDiags := model.RegionToProxyServerID.ElementsAs(ctx, &regionProxyIDs, false)
		diags.Append(mapDiags...)
		if diags.HasError() {
			return forward.CloudAccountRequest{}, diags
		}
	}

	collect := true
	if !model.Collect.IsNull() && !model.Collect.IsUnknown() {
		collect = model.Collect.ValueBool()
	}
	useForwardAccount := true
	if !model.UseForwardAccountToAssumeRole.IsNull() && !model.UseForwardAccountToAssumeRole.IsUnknown() {
		useForwardAccount = model.UseForwardAccountToAssumeRole.ValueBool()
	}
	mode, modeDiags := resolveAWSCredentialMode(model)
	diags.Append(modeDiags...)
	if diags.HasError() {
		return forward.CloudAccountRequest{}, diags
	}
	useForwardAccount = mode == awsCredentialModeForwardAssumeRole

	request := forward.CloudAccountRequest{
		Type:                          "AWS",
		Name:                          strings.TrimSpace(model.Name.ValueString()),
		Collect:                       &collect,
		Regions:                       selectedRegionInstants(regions),
		RegionToProxyServerID:         regionProxyIDs,
		AssumeRoleInfos:               assumeRoleInfos,
		UseForwardAccountToAssumeRole: &useForwardAccount,
		ProxyServerID:                 stringPointer(model.ProxyServerID),
		Concurrency:                   int64Pointer(model.Concurrency),
		ConnectionTimeoutSeconds:      int64Pointer(model.ConnectionTimeoutSeconds),
		RequestTimeoutSeconds:         int64Pointer(model.RequestTimeoutSeconds),
	}
	if mode == awsCredentialModeStaticKeys {
		request.Username = strings.TrimSpace(attrStringValue(model.CollectorAccessKeyID))
		request.Password = attrStringValue(model.CollectorSecretAccessKey)
	}

	if request.Name == "" {
		diags.AddAttributeError(path.Root("name"), "Missing Name", "name must be provided.")
	}

	return request, diags
}

func awsCloudAccountPatchRequest(request forward.CloudAccountRequest) forward.CloudAccountRequest {
	request.UseForwardAccountToAssumeRole = nil
	request.Username = ""
	request.Password = ""
	return request
}

func awsCloudAccountCredentialRequest(model awsCloudAccountResourceModel) forward.CloudAccountCredentialRequest {
	return forward.CloudAccountCredentialRequest{
		Type:     "AWS",
		Username: strings.TrimSpace(attrStringValue(model.CollectorAccessKeyID)),
		Password: attrStringValue(model.CollectorSecretAccessKey),
	}
}

func resolveAWSCredentialMode(model awsCloudAccountResourceModel) (string, diag.Diagnostics) {
	var diags diag.Diagnostics
	mode := strings.TrimSpace(attrStringValue(model.CredentialMode))
	if mode == "" {
		hasAccessKey := strings.TrimSpace(attrStringValue(model.CollectorAccessKeyID)) != ""
		hasSecretKey := strings.TrimSpace(attrStringValue(model.CollectorSecretAccessKey)) != ""
		switch {
		case hasAccessKey || hasSecretKey:
			mode = awsCredentialModeStaticKeys
		case !model.UseForwardAccountToAssumeRole.IsNull() && !model.UseForwardAccountToAssumeRole.IsUnknown() && !model.UseForwardAccountToAssumeRole.ValueBool():
			mode = awsCredentialModeInstanceProfile
		default:
			mode = awsCredentialModeForwardAssumeRole
		}
	}

	switch mode {
	case awsCredentialModeForwardAssumeRole, awsCredentialModeStaticKeys, awsCredentialModeInstanceProfile:
	default:
		diags.AddAttributeError(
			path.Root("credential_mode"),
			"Invalid AWS Credential Mode",
			fmt.Sprintf("credential_mode must be one of %q, %q, or %q.", awsCredentialModeForwardAssumeRole, awsCredentialModeStaticKeys, awsCredentialModeInstanceProfile),
		)
		return "", diags
	}

	accessKey := strings.TrimSpace(attrStringValue(model.CollectorAccessKeyID))
	secretKey := strings.TrimSpace(attrStringValue(model.CollectorSecretAccessKey))
	if mode == awsCredentialModeStaticKeys {
		if accessKey == "" {
			diags.AddAttributeError(path.Root("collector_access_key_id"), "Missing Collector Access Key ID", "collector_access_key_id is required when credential_mode is static-keys.")
		}
		if secretKey == "" {
			diags.AddAttributeError(path.Root("collector_secret_access_key"), "Missing Collector Secret Access Key", "collector_secret_access_key is required when credential_mode is static-keys.")
		}
	} else if accessKey != "" || secretKey != "" {
		diags.AddAttributeError(
			path.Root("credential_mode"),
			"Collector Keys Require Static-Key Mode",
			fmt.Sprintf("collector_access_key_id and collector_secret_access_key are only valid when credential_mode is %q.", awsCredentialModeStaticKeys),
		)
	}

	return mode, diags
}

func validateExistingAWSCredentialMode(existing *forward.CloudAccount, desiredMode string) error {
	if existing == nil || existing.UseForwardAccountToAssumeRole == nil {
		return nil
	}
	existingUsesForward := *existing.UseForwardAccountToAssumeRole
	desiredUsesForward := desiredMode == awsCredentialModeForwardAssumeRole
	if existingUsesForward != desiredUsesForward {
		return fmt.Errorf("existing setup %q has useForwardAccountToAssumeRole=%t, but credential_mode %q requires useForwardAccountToAssumeRole=%t", existing.Name, existingUsesForward, desiredMode, desiredUsesForward)
	}
	return nil
}

func accountRemovalDiagnostics(model awsCloudAccountResourceModel, existing *forward.CloudAccount, planned []forward.AWSAssumeRoleInfo) diag.Diagnostics {
	var diags diag.Diagnostics
	if existing == nil || (!model.AllowAccountRemovals.IsNull() && !model.AllowAccountRemovals.IsUnknown() && model.AllowAccountRemovals.ValueBool()) {
		return diags
	}

	plannedByID := map[string]bool{}
	for _, info := range planned {
		accountID := awsAssumeRoleInfoAccountID(info)
		if accountID != "" {
			plannedByID[accountID] = true
		}
	}

	removed := make([]string, 0)
	for _, info := range existing.AssumeRoleInfos {
		accountID := awsAssumeRoleInfoAccountID(info)
		if accountID == "" || plannedByID[accountID] {
			continue
		}
		removed = append(removed, awsAssumeRoleInfoLabel(info))
	}
	if len(removed) == 0 {
		return diags
	}

	sort.Strings(removed)
	diags.AddAttributeError(
		path.Root("allow_account_removals"),
		"AWS Account Removals Require Confirmation",
		fmt.Sprintf(
			"Forward setup %q currently includes %d AWS account entries, but this Terraform plan would remove %d: %s. If those removals are intentional, set allow_account_removals = true and re-run Terraform.",
			existing.Name,
			len(existing.AssumeRoleInfos),
			len(removed),
			strings.Join(removed, ", "),
		),
	)
	return diags
}

func awsAssumeRoleInfoAccountID(info forward.AWSAssumeRoleInfo) string {
	accountID := strings.TrimSpace(info.AccountID)
	if accountID != "" {
		return accountID
	}
	return accountIDFromRoleARN(info.RoleARN)
}

func awsAssumeRoleInfoLabel(info forward.AWSAssumeRoleInfo) string {
	accountID := awsAssumeRoleInfoAccountID(info)
	accountName := strings.TrimSpace(info.AccountName)
	if accountName != "" && accountID != "" && accountName != accountID {
		return fmt.Sprintf("%s (%s)", accountID, accountName)
	}
	if accountID != "" {
		return accountID
	}
	return strings.TrimSpace(info.RoleARN)
}

func accountIDFromRoleARN(roleARN string) string {
	parts := strings.Split(strings.TrimSpace(roleARN), ":")
	if len(parts) < 6 || parts[0] != "arn" || parts[2] != "iam" {
		return ""
	}
	return parts[4]
}

func credentialModeFromStateAndAPI(state awsCloudAccountResourceModel, account *forward.CloudAccount) string {
	if account != nil && account.UseForwardAccountToAssumeRole != nil && *account.UseForwardAccountToAssumeRole {
		return awsCredentialModeForwardAssumeRole
	}
	currentMode := strings.TrimSpace(attrStringValue(state.CredentialMode))
	if currentMode == awsCredentialModeStaticKeys {
		return awsCredentialModeStaticKeys
	}
	if account != nil && account.UseForwardAccountToAssumeRole != nil && !*account.UseForwardAccountToAssumeRole {
		return awsCredentialModeInstanceProfile
	}
	if currentMode == awsCredentialModeInstanceProfile {
		return awsCredentialModeInstanceProfile
	}
	return awsCredentialModeForwardAssumeRole
}

func expandAWSAssumeRoleInfos(values []awsAssumeRoleInfoModel) ([]forward.AWSAssumeRoleInfo, diag.Diagnostics) {
	var diags diag.Diagnostics
	result := make([]forward.AWSAssumeRoleInfo, 0, len(values))
	seen := map[string]bool{}

	for idx, value := range values {
		accountID := strings.TrimSpace(attrStringValue(value.AccountID))
		roleArn := strings.TrimSpace(attrStringValue(value.RoleArn))
		if accountID == "" {
			diags.AddAttributeError(path.Root("assume_role_infos").AtListIndex(idx).AtName("account_id"), "Missing Account ID", "account_id must be provided.")
			continue
		}
		if seen[accountID] {
			diags.AddAttributeError(path.Root("assume_role_infos").AtListIndex(idx).AtName("account_id"), "Duplicate Account ID", fmt.Sprintf("account_id %q appears more than once.", accountID))
			continue
		}
		seen[accountID] = true
		if roleArn == "" {
			diags.AddAttributeError(path.Root("assume_role_infos").AtListIndex(idx).AtName("role_arn"), "Missing Role ARN", "role_arn must be provided.")
			continue
		}
		enabled := true
		if !value.Enabled.IsNull() && !value.Enabled.IsUnknown() {
			enabled = value.Enabled.ValueBool()
		}
		result = append(result, forward.AWSAssumeRoleInfo{
			AccountID:   accountID,
			AccountName: strings.TrimSpace(attrStringValue(value.AccountName)),
			RoleARN:     roleArn,
			ExternalID:  strings.TrimSpace(attrStringValue(value.ExternalID)),
			Enabled:     enabled,
			ErrorMsg:    strings.TrimSpace(attrStringValue(value.ErrorMsg)),
		})
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].AccountID < result[j].AccountID
	})

	return result, diags
}

func flattenAWSAssumeRoleInfos(values []forward.AWSAssumeRoleInfo) []awsAssumeRoleInfoModel {
	result := make([]awsAssumeRoleInfoModel, 0, len(values))
	for _, value := range values {
		result = append(result, awsAssumeRoleInfoModel{
			AccountID:   stringOrNull(value.AccountID),
			AccountName: stringOrNull(value.AccountName),
			RoleArn:     stringOrNull(value.RoleARN),
			ExternalID:  stringOrNull(value.ExternalID),
			Enabled:     types.BoolValue(value.Enabled),
			ErrorMsg:    stringOrNull(value.ErrorMsg),
		})
	}
	sort.Slice(result, func(i, j int) bool {
		return attrStringValue(result[i].AccountID) < attrStringValue(result[j].AccountID)
	})
	return result
}

func selectedRegionInstants(regions []string) map[string]int64 {
	sort.Strings(regions)
	now := time.Now().UnixMilli()
	result := make(map[string]int64, len(regions))
	for _, region := range regions {
		region = strings.TrimSpace(region)
		if region != "" {
			result[region] = now
		}
	}
	return result
}

func regionNames(regions map[string]forward.Region) []string {
	result := make([]string, 0, len(regions))
	for region := range regions {
		if strings.TrimSpace(region) != "" {
			result = append(result, region)
		}
	}
	sort.Strings(result)
	return result
}

func stringSet(set types.Set) []string {
	if set.IsNull() || set.IsUnknown() {
		return nil
	}
	var values []string
	for _, elem := range set.Elements() {
		if str, ok := elem.(basetypes.StringValue); ok {
			value := strings.TrimSpace(str.ValueString())
			if value != "" {
				values = append(values, value)
			}
		}
	}
	sort.Strings(values)
	return values
}

func setOfStrings(values []string) types.Set {
	elements := make([]attr.Value, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			elements = append(elements, types.StringValue(value))
		}
	}
	return types.SetValueMust(types.StringType, elements)
}

func mapOfStrings(ctx context.Context, values map[string]string) types.Map {
	if len(values) == 0 {
		return types.MapNull(types.StringType)
	}
	result, diags := types.MapValueFrom(ctx, types.StringType, values)
	if diags.HasError() {
		return types.MapNull(types.StringType)
	}
	return result
}

func stringPointer(value types.String) *string {
	if value.IsNull() || value.IsUnknown() {
		return nil
	}
	trimmed := strings.TrimSpace(value.ValueString())
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func int64Pointer(value types.Int64) *int64 {
	if value.IsNull() || value.IsUnknown() {
		return nil
	}
	v := value.ValueInt64()
	return &v
}

func payloadJSONForState(ctx context.Context, state awsCloudAccountResourceModel) types.String {
	collect := true
	if !state.Collect.IsNull() && !state.Collect.IsUnknown() {
		collect = state.Collect.ValueBool()
	}
	mode := strings.TrimSpace(attrStringValue(state.CredentialMode))
	if mode == "" {
		mode = awsCredentialModeForwardAssumeRole
	}
	regionToProxy := map[string]string{}
	if !state.RegionToProxyServerID.IsNull() && !state.RegionToProxyServerID.IsUnknown() {
		if mapDiags := state.RegionToProxyServerID.ElementsAs(ctx, &regionToProxy, false); mapDiags.HasError() {
			return types.StringNull()
		}
	}
	useForwardAccount := mode == awsCredentialModeForwardAssumeRole
	request := forward.CloudAccountRequest{
		Type:                          "AWS",
		Name:                          strings.TrimSpace(attrStringValue(state.Name)),
		Collect:                       &collect,
		ProxyServerID:                 stringPointer(state.ProxyServerID),
		Regions:                       selectedRegionInstants(stringSet(state.Regions)),
		RegionToProxyServerID:         regionToProxy,
		UseForwardAccountToAssumeRole: &useForwardAccount,
		Concurrency:                   int64Pointer(state.Concurrency),
		ConnectionTimeoutSeconds:      int64Pointer(state.ConnectionTimeoutSeconds),
		RequestTimeoutSeconds:         int64Pointer(state.RequestTimeoutSeconds),
	}
	infos, infoDiags := expandAWSAssumeRoleInfos(state.AssumeRoleInfos)
	if infoDiags.HasError() {
		return types.StringNull()
	}
	request.AssumeRoleInfos = infos
	if mode == awsCredentialModeStaticKeys {
		request.Username = redactedSecretValue
		request.Password = redactedSecretValue
	}
	payload, err := json.MarshalIndent(request, "", "  ")
	if err != nil {
		return types.StringNull()
	}
	return types.StringValue(string(payload))
}

func manualAccountJSONForState(infos []awsAssumeRoleInfoModel) types.String {
	entries := make([]manualAWSAccountData, 0, len(infos))
	for _, info := range infos {
		roleArn := strings.TrimSpace(attrStringValue(info.RoleArn))
		entry := manualAWSAccountData{
			ID:   strings.TrimSpace(attrStringValue(info.AccountID)),
			Name: strings.TrimSpace(attrStringValue(info.AccountName)),
		}
		if roleArn != "" {
			entry.RoleArn = &roleArn
		}
		externalID := strings.TrimSpace(attrStringValue(info.ExternalID))
		if externalID != "" {
			entry.ExternalID = &externalID
		}
		entries = append(entries, entry)
	}
	payload, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return types.StringNull()
	}
	return types.StringValue(string(payload))
}

func awsAssumeRoleInfoResourceAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"account_id": schema.StringAttribute{
			MarkdownDescription: "AWS account ID.",
			Required:            true,
		},
		"account_name": schema.StringAttribute{
			MarkdownDescription: "AWS account name.",
			Optional:            true,
			Computed:            true,
		},
		"role_arn": schema.StringAttribute{
			MarkdownDescription: "IAM role ARN Forward should assume for this account.",
			Required:            true,
		},
		"external_id": schema.StringAttribute{
			MarkdownDescription: "External ID supplied to STS AssumeRole.",
			Optional:            true,
			Computed:            true,
		},
		"enabled": schema.BoolAttribute{
			MarkdownDescription: "Whether Forward collection is enabled for this account.",
			Optional:            true,
			Computed:            true,
			Default:             booldefault.StaticBool(true),
		},
		"error_msg": schema.StringAttribute{
			MarkdownDescription: "Forward collection error message, when returned by the API.",
			Computed:            true,
		},
	}
}
