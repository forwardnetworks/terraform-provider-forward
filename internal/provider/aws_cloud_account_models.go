// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import "github.com/hashicorp/terraform-plugin-framework/types"

type awsAssumeRoleInfoModel struct {
	AccountID   types.String `tfsdk:"account_id"`
	AccountName types.String `tfsdk:"account_name"`
	RoleArn     types.String `tfsdk:"role_arn"`
	ExternalID  types.String `tfsdk:"external_id"`
	Enabled     types.Bool   `tfsdk:"enabled"`
	ErrorMsg    types.String `tfsdk:"error_msg"`
}
