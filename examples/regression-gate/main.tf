# Copyright (c) HashiCorp, Inc.

terraform {
  required_providers { forward = { source = "forwardnetworks/forward" } }
}

variable "network_id" { type = string }

# Credentials come from FORWARD_USERNAME / FORWARD_PASSWORD.
provider "forward" {
  network_id = var.network_id
}

variable "cloud_source" { type = string }
variable "route_table" { type = string }
variable "destination" { type = string }
variable "target_kind" { type = string }
variable "target_id" { type = string }

# Predict the change against the collected baseline. The regression suite is
# persistent, so Forward evaluates all nine checks on the predicted network.
resource "forward_predicted_snapshot" "candidate" {
  network_id  = var.network_id
  source_name = var.cloud_source
  note        = "gate demo"

  cloud_changes_json = jsonencode({
    routeChanges = [{
      routeTableId         = var.route_table
      destinationCidrBlock = var.destination
      target               = { kind = var.target_kind, id = var.target_id }
    }]
  })
}

# What the suite says about the network as it is today. A network is never
# perfectly clean, so the gate asks about regression -- what this change breaks
# -- not about absolute health, which would refuse every change forever.
data "forward_intent_checks" "baseline" {
  snapshot_id = forward_predicted_snapshot.candidate.base_snapshot_id
}

# THE GATE. Not a check block -- a postcondition, so a failure is an error.
data "forward_intent_checks" "predicted" {
  snapshot_id = forward_predicted_snapshot.candidate.id

  lifecycle {
    # Both sides read self, not the data source's own address: referring to
    # the resource being validated is a dependency cycle.
    postcondition {
      condition = length([
        for check in self.checks : check.name
        if check.status == "FAIL" && contains(local.passing_today, check.name)
      ]) == 0
      error_message = format("This change breaks %d check(s) that pass today: %s",
        length([for check in self.checks : check.name
        if check.status == "FAIL" && contains(local.passing_today, check.name)]),
        join(", ", sort([for check in self.checks : check.name
        if check.status == "FAIL" && contains(local.passing_today, check.name)])),
      )
    }
  }
}

locals {
  passing_today = toset([
    for check in data.forward_intent_checks.baseline.checks : check.name if check.status == "PASS"
  ])
}

resource "terraform_data" "the_change" {
  input      = var.destination
  depends_on = [data.forward_intent_checks.predicted]
}

output "predicted_snapshot" { value = forward_predicted_snapshot.candidate.id }
output "failing_today" { value = data.forward_intent_checks.baseline.fail_count }
output "failing_predicted" { value = data.forward_intent_checks.predicted.fail_count }
output "newly_failing" {
  value = sort([for check in data.forward_intent_checks.predicted.checks :
  check.name if check.status == "FAIL" && contains(local.passing_today, check.name)])
}
