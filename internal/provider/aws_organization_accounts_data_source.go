// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/organizations"
	organizationstypes "github.com/aws/aws-sdk-go-v2/service/organizations/types"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &AWSOrganizationAccountsDataSource{}

// NewAWSOrganizationAccountsDataSource instantiates the AWS Organizations account discovery data source.
func NewAWSOrganizationAccountsDataSource() datasource.DataSource {
	return &AWSOrganizationAccountsDataSource{}
}

// AWSOrganizationAccountsDataSource discovers AWS Organizations accounts and builds Forward assume-role entries.
type AWSOrganizationAccountsDataSource struct{}

type awsOrganizationAccountsDataSourceModel struct {
	RoleName         types.String `tfsdk:"role_name"`
	ExternalID       types.String `tfsdk:"external_id"`
	AWSProfile       types.String `tfsdk:"aws_profile"`
	AWSRegion        types.String `tfsdk:"aws_region"`
	IncludeSuspended types.Bool   `tfsdk:"include_suspended"`
	IncludeAccountID types.List   `tfsdk:"include_account_ids"`
	ExcludeAccountID types.List   `tfsdk:"exclude_account_ids"`

	OrganizationID      types.String                  `tfsdk:"organization_id"`
	OrganizationARN     types.String                  `tfsdk:"organization_arn"`
	ManagementAccountID types.String                  `tfsdk:"management_account_id"`
	FeatureSet          types.String                  `tfsdk:"feature_set"`
	AccountCount        types.Int64                   `tfsdk:"account_count"`
	SkippedAccountCount types.Int64                   `tfsdk:"skipped_account_count"`
	Accounts            []awsOrganizationAccountModel `tfsdk:"accounts"`
	AssumeRoleInfos     []awsAssumeRoleInfoModel      `tfsdk:"assume_role_infos"`
	ManualAccountJSON   types.String                  `tfsdk:"manual_account_data_json"`
}

type awsOrganizationAccountModel struct {
	ID        types.String `tfsdk:"id"`
	Name      types.String `tfsdk:"name"`
	Email     types.String `tfsdk:"email"`
	State     types.String `tfsdk:"state"`
	ParentIDs types.List   `tfsdk:"parent_ids"`
}

type manualAWSAccountData struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	RoleArn    *string `json:"roleArn,omitempty"`
	ExternalID *string `json:"externalId,omitempty"`
}

func (d *AWSOrganizationAccountsDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_aws_organization_accounts"
}

func (d *AWSOrganizationAccountsDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Discover AWS Organizations accounts using the AWS SDK credential chain and build Forward assume-role account entries.",
		Attributes: map[string]schema.Attribute{
			"role_name": schema.StringAttribute{
				MarkdownDescription: "Stable IAM role name that exists in every AWS account Forward should collect.",
				Required:            true,
			},
			"external_id": schema.StringAttribute{
				MarkdownDescription: "External ID to include in every assume-role entry. Use the forward_aws_assume_role_external_id data source when Forward should generate it.",
				Optional:            true,
			},
			"aws_profile": schema.StringAttribute{
				MarkdownDescription: "Optional shared AWS config profile for Organizations discovery. When omitted, the AWS SDK default credential chain is used.",
				Optional:            true,
			},
			"aws_region": schema.StringAttribute{
				MarkdownDescription: "AWS region used to configure the Organizations client. Defaults to us-east-1 when omitted.",
				Optional:            true,
				Computed:            true,
			},
			"include_suspended": schema.BoolAttribute{
				MarkdownDescription: "Include non-ACTIVE AWS accounts. Defaults to false.",
				Optional:            true,
			},
			"include_account_ids": schema.ListAttribute{
				MarkdownDescription: "Optional allow-list of AWS account IDs to include.",
				ElementType:         types.StringType,
				Optional:            true,
			},
			"exclude_account_ids": schema.ListAttribute{
				MarkdownDescription: "Optional deny-list of AWS account IDs to exclude.",
				ElementType:         types.StringType,
				Optional:            true,
			},
			"organization_id": schema.StringAttribute{
				MarkdownDescription: "AWS Organizations organization ID.",
				Computed:            true,
			},
			"organization_arn": schema.StringAttribute{
				MarkdownDescription: "AWS Organizations organization ARN.",
				Computed:            true,
			},
			"management_account_id": schema.StringAttribute{
				MarkdownDescription: "AWS Organizations management account ID.",
				Computed:            true,
			},
			"feature_set": schema.StringAttribute{
				MarkdownDescription: "AWS Organizations feature set.",
				Computed:            true,
			},
			"account_count": schema.Int64Attribute{
				MarkdownDescription: "Number of accounts emitted after filters.",
				Computed:            true,
			},
			"skipped_account_count": schema.Int64Attribute{
				MarkdownDescription: "Number of accounts skipped because of state or include/exclude filters.",
				Computed:            true,
			},
			"accounts": schema.ListNestedAttribute{
				MarkdownDescription: "AWS Organizations accounts emitted after filters.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":    schema.StringAttribute{Computed: true},
						"name":  schema.StringAttribute{Computed: true},
						"email": schema.StringAttribute{Computed: true},
						"state": schema.StringAttribute{Computed: true},
						"parent_ids": schema.ListAttribute{
							ElementType: types.StringType,
							Computed:    true,
						},
					},
				},
			},
			"assume_role_infos": schema.ListNestedAttribute{
				MarkdownDescription: "Forward AWS assume-role entries derived from the discovered AWS accounts and role_name.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: awsAssumeRoleInfoDataSourceAttributes(),
				},
			},
			"manual_account_data_json": schema.StringAttribute{
				MarkdownDescription: "JSON array compatible with Forward's manual AWS account upload flow.",
				Computed:            true,
			},
		},
	}
}

func (d *AWSOrganizationAccountsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data awsOrganizationAccountsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	roleName := strings.TrimSpace(data.RoleName.ValueString())
	if roleName == "" {
		resp.Diagnostics.AddAttributeError(path.Root("role_name"), "Missing Role Name", "role_name must be provided.")
		return
	}

	awsRegion := strings.TrimSpace(attrStringValue(data.AWSRegion))
	if awsRegion == "" {
		awsRegion = "us-east-1"
	}
	data.AWSRegion = types.StringValue(awsRegion)

	discovery, err := discoverAWSOrganizationAccounts(ctx, awsOrganizationDiscoveryOptions{
		Profile:          attrStringValue(data.AWSProfile),
		Region:           awsRegion,
		RoleName:         roleName,
		ExternalID:       attrStringValue(data.ExternalID),
		IncludeSuspended: !data.IncludeSuspended.IsNull() && data.IncludeSuspended.ValueBool(),
		IncludeAccountID: trimmedSet(stringList(data.IncludeAccountID)),
		ExcludeAccountID: trimmedSet(stringList(data.ExcludeAccountID)),
	})
	if err != nil {
		resp.Diagnostics.AddError("Unable to Discover AWS Organization Accounts", err.Error())
		return
	}

	data.OrganizationID = stringOrNull(discovery.OrganizationID)
	data.OrganizationARN = stringOrNull(discovery.OrganizationARN)
	data.ManagementAccountID = stringOrNull(discovery.ManagementAccountID)
	data.FeatureSet = stringOrNull(discovery.FeatureSet)
	data.AccountCount = types.Int64Value(int64(len(discovery.Accounts)))
	data.SkippedAccountCount = types.Int64Value(int64(discovery.SkippedAccountCount))
	data.Accounts = make([]awsOrganizationAccountModel, 0, len(discovery.Accounts))
	data.AssumeRoleInfos = make([]awsAssumeRoleInfoModel, 0, len(discovery.Accounts))

	manualEntries := make([]manualAWSAccountData, 0, len(discovery.Accounts))
	for _, account := range discovery.Accounts {
		data.Accounts = append(data.Accounts, awsOrganizationAccountModel{
			ID:        types.StringValue(account.ID),
			Name:      types.StringValue(account.Name),
			Email:     stringOrNull(account.Email),
			State:     stringOrNull(account.State),
			ParentIDs: listOfStrings(account.ParentIDs),
		})
		info := awsAssumeRoleInfoModel{
			AccountID:   types.StringValue(account.ID),
			AccountName: types.StringValue(account.Name),
			RoleArn:     types.StringValue(account.RoleArn),
			ExternalID:  stringOrNull(discovery.ExternalID),
			Enabled:     types.BoolValue(true),
			ErrorMsg:    types.StringNull(),
		}
		data.AssumeRoleInfos = append(data.AssumeRoleInfos, info)

		roleArn := account.RoleArn
		manualEntry := manualAWSAccountData{
			ID:      account.ID,
			Name:    account.Name,
			RoleArn: &roleArn,
		}
		if discovery.ExternalID != "" {
			externalID := discovery.ExternalID
			manualEntry.ExternalID = &externalID
		}
		manualEntries = append(manualEntries, manualEntry)
	}

	manualJSON, err := json.MarshalIndent(manualEntries, "", "  ")
	if err != nil {
		resp.Diagnostics.AddError("Unable to Build Manual Account JSON", err.Error())
		return
	}
	data.ManualAccountJSON = types.StringValue(string(manualJSON))

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

type awsOrganizationDiscoveryOptions struct {
	Profile          string
	Region           string
	RoleName         string
	ExternalID       string
	IncludeSuspended bool
	IncludeAccountID map[string]bool
	ExcludeAccountID map[string]bool
}

type awsOrganizationDiscovery struct {
	OrganizationID      string
	OrganizationARN     string
	ManagementAccountID string
	FeatureSet          string
	ExternalID          string
	SkippedAccountCount int
	Accounts            []awsOrganizationDiscoveredAccount
}

type awsOrganizationDiscoveredAccount struct {
	ID        string
	Name      string
	Email     string
	State     string
	ParentIDs []string
	RoleArn   string
}

func discoverAWSOrganizationAccounts(ctx context.Context, opts awsOrganizationDiscoveryOptions) (*awsOrganizationDiscovery, error) {
	if strings.TrimSpace(opts.RoleName) == "" {
		return nil, fmt.Errorf("role name must be provided")
	}
	region := strings.TrimSpace(opts.Region)
	if region == "" {
		region = "us-east-1"
	}

	loadOptions := []func(*config.LoadOptions) error{config.WithRegion(region)}
	if strings.TrimSpace(opts.Profile) != "" {
		loadOptions = append(loadOptions, config.WithSharedConfigProfile(strings.TrimSpace(opts.Profile)))
	}

	awsConfig, err := config.LoadDefaultConfig(ctx, loadOptions...)
	if err != nil {
		return nil, fmt.Errorf("load AWS configuration: %w", err)
	}

	client := organizations.NewFromConfig(awsConfig)
	org, err := client.DescribeOrganization(ctx, &organizations.DescribeOrganizationInput{})
	if err != nil {
		return nil, fmt.Errorf("describe organization: %w", err)
	}
	if org.Organization == nil {
		return nil, fmt.Errorf("describe organization returned no organization")
	}

	result := &awsOrganizationDiscovery{
		OrganizationID:      aws.ToString(org.Organization.Id),
		OrganizationARN:     aws.ToString(org.Organization.Arn),
		ManagementAccountID: aws.ToString(org.Organization.MasterAccountId),
		FeatureSet:          string(org.Organization.FeatureSet),
		ExternalID:          strings.TrimSpace(opts.ExternalID),
	}

	paginator := organizations.NewListAccountsPaginator(client, &organizations.ListAccountsInput{})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list organization accounts: %w", err)
		}
		for _, account := range page.Accounts {
			discovered, skip, err := buildDiscoveredAWSAccount(ctx, client, account, opts)
			if err != nil {
				return nil, err
			}
			if skip {
				result.SkippedAccountCount++
				continue
			}
			if discovered.ID == "" {
				result.SkippedAccountCount++
				continue
			}
			result.Accounts = append(result.Accounts, discovered)
		}
	}

	sort.Slice(result.Accounts, func(i, j int) bool {
		return result.Accounts[i].ID < result.Accounts[j].ID
	})

	if len(result.Accounts) == 0 {
		return nil, fmt.Errorf("AWS Organizations discovery returned no accounts after filters")
	}

	return result, nil
}

func buildDiscoveredAWSAccount(ctx context.Context, client *organizations.Client, account organizationstypes.Account, opts awsOrganizationDiscoveryOptions) (awsOrganizationDiscoveredAccount, bool, error) {
	accountID := strings.TrimSpace(aws.ToString(account.Id))
	if accountID == "" {
		return awsOrganizationDiscoveredAccount{}, true, nil
	}
	if len(opts.IncludeAccountID) > 0 && !opts.IncludeAccountID[accountID] {
		return awsOrganizationDiscoveredAccount{}, true, nil
	}
	if opts.ExcludeAccountID[accountID] {
		return awsOrganizationDiscoveredAccount{}, true, nil
	}

	state := accountState(account)
	if !opts.IncludeSuspended && state != string(organizationstypes.AccountStateActive) {
		return awsOrganizationDiscoveredAccount{}, true, nil
	}

	parents, err := client.ListParents(ctx, &organizations.ListParentsInput{
		ChildId: &accountID,
	})
	if err != nil {
		return awsOrganizationDiscoveredAccount{}, false, fmt.Errorf("list parents for AWS account %s: %w", accountID, err)
	}

	parentIDs := make([]string, 0, len(parents.Parents))
	for _, parent := range parents.Parents {
		parentID := strings.TrimSpace(aws.ToString(parent.Id))
		if parentID != "" {
			parentIDs = append(parentIDs, parentID)
		}
	}
	sort.Strings(parentIDs)

	accountName := strings.TrimSpace(aws.ToString(account.Name))
	if accountName == "" {
		accountName = accountID
	}

	return awsOrganizationDiscoveredAccount{
		ID:        accountID,
		Name:      accountName,
		Email:     aws.ToString(account.Email),
		State:     state,
		ParentIDs: parentIDs,
		RoleArn:   fmt.Sprintf("arn:aws:iam::%s:role/%s", accountID, strings.TrimSpace(opts.RoleName)),
	}, false, nil
}

func accountState(account organizationstypes.Account) string {
	if account.State != "" {
		return string(account.State)
	}
	if account.Status != "" {
		return string(account.Status)
	}
	return ""
}

func trimmedSet(values []string) map[string]bool {
	result := make(map[string]bool, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			result[value] = true
		}
	}
	return result
}

func awsAssumeRoleInfoDataSourceAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"account_id": schema.StringAttribute{
			MarkdownDescription: "AWS account ID.",
			Computed:            true,
		},
		"account_name": schema.StringAttribute{
			MarkdownDescription: "AWS account name.",
			Computed:            true,
		},
		"role_arn": schema.StringAttribute{
			MarkdownDescription: "IAM role ARN Forward should assume for this account.",
			Computed:            true,
		},
		"external_id": schema.StringAttribute{
			MarkdownDescription: "External ID supplied to STS AssumeRole.",
			Computed:            true,
		},
		"enabled": schema.BoolAttribute{
			MarkdownDescription: "Whether Forward collection is enabled for this account.",
			Computed:            true,
		},
		"error_msg": schema.StringAttribute{
			MarkdownDescription: "Forward collection error message, when returned by the API.",
			Computed:            true,
		},
	}
}
