# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

variable "forward_collection_role_name" {
  description = "Stable IAM role name deployed to each AWS account."
  type        = string
}

variable "aws_profile" {
  description = "Optional AWS profile with organizations:DescribeOrganization, organizations:ListAccounts, and organizations:ListParents."
  type        = string
  default     = null
}

data "forward_aws_assume_role_external_id" "current" {}

data "forward_aws_organization_accounts" "current" {
  role_name   = var.forward_collection_role_name
  external_id = data.forward_aws_assume_role_external_id.current.external_id
  aws_profile = var.aws_profile
}

output "forward_assume_role_infos" {
  value = data.forward_aws_organization_accounts.current.assume_role_infos
}

output "manual_account_data_json" {
  value = data.forward_aws_organization_accounts.current.manual_account_data_json
}
