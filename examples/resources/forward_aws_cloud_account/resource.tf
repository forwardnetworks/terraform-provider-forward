# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

variable "forward_aws_setup_name" {
  description = "Forward AWS cloud setup name."
  type        = string
}

variable "forward_collection_role_name" {
  description = "Stable IAM role name deployed to each AWS account."
  type        = string
}

variable "forward_collection_regions" {
  description = "AWS regions Forward should collect."
  type        = set(string)
}

variable "forward_credential_mode" {
  description = "Forward AWS collection credential model: forward-assume-role, static-keys, or instance-profile."
  type        = string
  default     = "forward-assume-role"
}

variable "assume_role_external_id" {
  description = "Optional external ID for static-keys or instance-profile mode."
  type        = string
  default     = null
}

variable "forward_collector_access_key_id" {
  description = "AWS access key ID stored in Forward when using static-keys mode."
  type        = string
  default     = null
  sensitive   = true
}

variable "forward_collector_secret_access_key" {
  description = "AWS secret access key stored in Forward when using static-keys mode."
  type        = string
  default     = null
  sensitive   = true
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
}

resource "forward_aws_cloud_account" "organization" {
  name    = var.forward_aws_setup_name
  collect = true
  regions = var.forward_collection_regions

  credential_mode             = var.forward_credential_mode
  collector_access_key_id     = var.forward_credential_mode == "static-keys" ? var.forward_collector_access_key_id : null
  collector_secret_access_key = var.forward_credential_mode == "static-keys" ? var.forward_collector_secret_access_key : null

  # Defaults to false. Set true only after reviewing a plan that intentionally removes accounts.
  # allow_account_removals = true

  assume_role_infos = data.forward_aws_organization_accounts.current.assume_role_infos
}
