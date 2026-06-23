# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

variable "forward_username" {
  description = "Forward username."
  type        = string
}

variable "forward_password" {
  description = "Forward password."
  type        = string
  sensitive   = true
}

variable "forward_base_url" {
  description = "Forward Networks API base URL."
  type        = string
}

variable "forward_network_id" {
  description = "Forward Networks network identifier."
  type        = string
}

variable "forward_aws_setup_name" {
  description = "Forward AWS cloud setup name to create or update."
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

  validation {
    condition     = contains(["forward-assume-role", "static-keys", "instance-profile"], var.forward_credential_mode)
    error_message = "forward_credential_mode must be forward-assume-role, static-keys, or instance-profile."
  }
}

variable "assume_role_external_id" {
  description = "Optional external ID to pass to STS AssumeRole for static-keys or instance-profile mode when the AWS role trust policy requires one."
  type        = string
  default     = null
}

variable "forward_collector_access_key_id" {
  description = "AWS access key ID stored in Forward when forward_credential_mode is static-keys."
  type        = string
  default     = null
  sensitive   = true
}

variable "forward_collector_secret_access_key" {
  description = "AWS secret access key stored in Forward when forward_credential_mode is static-keys."
  type        = string
  default     = null
  sensitive   = true
}

variable "aws_profile" {
  description = "Optional AWS profile with Organizations read permissions."
  type        = string
  default     = null
}

variable "aws_region" {
  description = "AWS region used to configure the Organizations client."
  type        = string
  default     = "us-east-1"
}
