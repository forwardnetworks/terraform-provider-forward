# Refuse a change Forward predicts will break reachability.
#
# The prediction is made from the plan, before the change exists. The check
# block reads the predicted network and fails the apply if the answer is wrong,
# so the gate runs in the same command as the change rather than as a report
# afterwards.

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
}

check "reachability_survives" {
  assert {
    condition     = data.forward_path_analysis.app_to_db.forwarding_outcome == "DELIVERED"
    error_message = "This change breaks app -> db: Forward predicts ${data.forward_path_analysis.app_to_db.forwarding_outcome}."
  }
}

output "predicted_snapshot" {
  value       = forward_predicted_snapshot.candidate.id
  description = "Snapshot of the network as it would be, for comparison with what is collected after the apply."
}
