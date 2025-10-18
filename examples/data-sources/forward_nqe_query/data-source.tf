# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

variable "forward_api_key" {
  description = "Forward Networks API key."
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

variable "forward_insecure" {
  description = "Disable TLS certificate verification."
  type        = bool
  default     = false
}

provider "forward" {
  base_url   = var.forward_base_url
  network_id = var.forward_network_id
  api_key    = var.forward_api_key
  insecure   = var.forward_insecure
}

data "forward_nqe_query" "access_list_entries" {
  snapshot_id = "snap-123"
  query_id    = "my-library-query"
  parameters = {
    filter = "\"critical\""
  }
  limit = 10
}
