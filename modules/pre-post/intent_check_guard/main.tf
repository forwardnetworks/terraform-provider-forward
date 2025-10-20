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

data "forward_intent_checks" "baseline" {
  snapshot_id = var.baseline_snapshot_id
  status      = var.failure_statuses
}

data "forward_intent_checks" "verification" {
  snapshot_id = var.verification_snapshot_id
  status      = var.failure_statuses
}

resource "null_resource" "guard" {
  lifecycle {
    precondition {
      condition     = data.forward_intent_checks.baseline.fail_count == 0
      error_message = "Baseline snapshot has failing intent checks."
    }

    postcondition {
      condition     = data.forward_intent_checks.verification.fail_count == 0
      error_message = "Verification snapshot has failing intent checks."
    }
  }
}

output "baseline_summary" {
  description = "Baseline intent check summary."
  value       = data.forward_intent_checks.baseline
}

output "verification_summary" {
  description = "Verification intent check summary."
  value       = data.forward_intent_checks.verification
}
