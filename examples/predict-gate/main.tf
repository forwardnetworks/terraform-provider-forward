# Copyright (c) HashiCorp, Inc.

# Refuse a change Forward predicts will break reachability.
#
# The prediction is made from the plan, before the change exists, and the gate
# runs in the same command as the change rather than as a report afterwards.
#
# The gate is a postcondition, not a check block. A failing check block is a
# warning: Terraform prints it and applies anyway, exit code 0. A failing
# postcondition is an error, and anything downstream of the gated data source
# is never created.

terraform {
  required_providers {
    forward = {
      source = "forwardnetworks/forward"
    }
  }
}

variable "network_id" {
  type        = string
  description = "Forward network holding the cloud source."
}

variable "cloud_source" {
  type        = string
  description = "Cloud collection source the change applies to."
}

variable "plan_json" {
  type        = string
  description = "Path to `terraform show -json` output for the change under review."
}

resource "forward_predicted_snapshot" "candidate" {
  network_id          = var.network_id
  source_name         = var.cloud_source
  terraform_plan_json = file(var.plan_json)
  note                = "terraform predict gate"
}

# Snapshot-scoped, so this asks about the network that does not exist yet.
data "forward_path_analysis" "app_to_db" {
  network_id  = var.network_id
  snapshot_id = forward_predicted_snapshot.candidate.id
  src_ip      = "10.49.1.10"
  dst_ip      = "10.51.1.10"

  lifecycle {
    postcondition {
      # forwarding_outcome is the outcome every path agrees on, so a change
      # that breaks one of several paths reports MIXED and fails here too.
      condition     = self.forwarding_outcome == "DELIVERED"
      error_message = "This change breaks app -> db: Forward predicts ${coalesce(self.forwarding_outcome, "no path at all")}."
    }
  }
}

output "predicted_snapshot" {
  value       = forward_predicted_snapshot.candidate.id
  description = "Snapshot of the network as it would be, for comparison with what is collected after the apply."
}
