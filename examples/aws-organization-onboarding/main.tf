# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

provider "forward" {
  base_url   = var.forward_base_url
  network_id = var.forward_network_id
  username   = var.forward_username
  password   = var.forward_password
}

data "forward_aws_assume_role_external_id" "current" {
  count = var.forward_credential_mode == "forward-assume-role" ? 1 : 0
}

locals {
  assume_role_external_id = var.forward_credential_mode == "forward-assume-role" ? data.forward_aws_assume_role_external_id.current[0].external_id : var.assume_role_external_id
}

data "forward_aws_organization_accounts" "current" {
  role_name   = var.forward_collection_role_name
  external_id = local.assume_role_external_id
  aws_profile = var.aws_profile
  aws_region  = var.aws_region
}

resource "forward_aws_cloud_account" "organization" {
  name    = var.forward_aws_setup_name
  collect = true
  regions = var.forward_collection_regions

  credential_mode             = var.forward_credential_mode
  collector_access_key_id     = var.forward_credential_mode == "static-keys" ? var.forward_collector_access_key_id : null
  collector_secret_access_key = var.forward_credential_mode == "static-keys" ? var.forward_collector_secret_access_key : null

  assume_role_infos = data.forward_aws_organization_accounts.current.assume_role_infos
}

output "aws_organization_id" {
  value = data.forward_aws_organization_accounts.current.organization_id
}

output "forward_account_count" {
  value = forward_aws_cloud_account.organization.account_count
}

output "manual_account_data_json" {
  value = data.forward_aws_organization_accounts.current.manual_account_data_json
}

output "forward_payload_json" {
  value = forward_aws_cloud_account.organization.payload_json
}
