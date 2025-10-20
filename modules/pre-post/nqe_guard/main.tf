# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

# Copyright (c) HashiCorp, Inc.

terraform {
  required_providers {
    forward = {
      source  = "forwardnetworks/forward"
      version = ">= 0.1"
    }
  }
}

data "forward_nqe_query" "baseline" {
  snapshot_id = var.baseline_snapshot_id
  query_id    = var.query_id
  query       = var.inline_query
  limit       = var.limit
}

data "forward_nqe_query" "verification" {
  snapshot_id = var.verification_snapshot_id
  query_id    = var.query_id
  query       = var.inline_query
  limit       = var.limit
}

resource "null_resource" "guard" {
  lifecycle {
    precondition {
      condition     = length(data.forward_nqe_query.baseline.items_json) == 0
      error_message = var.baseline_error_message
    }

    postcondition {
      condition     = length(data.forward_nqe_query.verification.items_json) == 0
      error_message = var.verification_error_message
    }
  }
}

output "baseline_results" {
  value = data.forward_nqe_query.baseline.items_json
}

output "verification_results" {
  value = data.forward_nqe_query.verification.items_json
}
