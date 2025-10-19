# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

terraform {
  required_providers {
    forward = {
      source  = "forwardnetworks/forward"
      version = "~> 0.1"
    }
  }
}

provider "forward" {
  base_url   = var.forward_base_url
  network_id = var.forward_network_id
  api_key    = var.forward_api_key
  insecure   = var.forward_insecure
}

############################################
# Snapshot Intent Check Pre/Post Validation #
############################################

# Create (or re-assert) an intent check against the baseline snapshot. The
# definition must match the Forward Enterprise API schema for NewNetworkCheck.
resource "forward_intent_check" "baseline" {
  snapshot_id     = var.baseline_snapshot_id
  name            = var.intent_check_name
  definition_json = var.intent_check_definition
  persistent      = false
}

data "forward_intent_checks" "baseline_summary" {
  snapshot_id = forward_intent_check.baseline.snapshot_id
  status      = ["FAIL", "ERROR", "TIMEOUT"]
}

resource "null_resource" "baseline_guard" {
  lifecycle {
    precondition {
      condition     = data.forward_intent_checks.baseline_summary.fail_count == 0
      error_message = "Baseline snapshot has failing intent checks. Investigate before continuing."
    }
  }
}

data "forward_intent_checks" "verification_summary" {
  snapshot_id = var.verification_snapshot_id
  status      = ["FAIL", "ERROR", "TIMEOUT"]
}

resource "null_resource" "verification_guard" {
  lifecycle {
    precondition {
      condition     = data.forward_intent_checks.verification_summary.fail_count == 0
      error_message = "Verification snapshot reports failing intent checks."
    }
  }
}

############################
# NQE Drift Verification   #
############################

resource "forward_nqe_query_definition" "drift" {
  path       = var.nqe_query_path
  repository = var.nqe_repository
}

data "forward_nqe_query" "baseline" {
  snapshot_id = var.baseline_snapshot_id
  query_id    = forward_nqe_query_definition.drift.query_id
  limit       = var.nqe_query_limit
}

data "forward_nqe_query" "verification" {
  snapshot_id = var.verification_snapshot_id
  query_id    = forward_nqe_query_definition.drift.query_id
  limit       = var.nqe_query_limit
}

resource "null_resource" "nqe_guard" {
  lifecycle {
    precondition {
      condition     = length(data.forward_nqe_query.baseline.items_json) == 0
      error_message = "Baseline NQE query returned unexpected results."
    }

    postcondition {
      condition     = length(data.forward_nqe_query.verification.items_json) == 0
      error_message = "Verification snapshot NQE query returned results indicating drift."
    }
  }
}
