# Copyright (c) HashiCorp, Inc.

# Collect, predict, gate, change, collect again, gate again -- one apply.
#
# The two gates are postconditions. A failing check block is a warning and the
# apply proceeds; a failing postcondition is an error and everything downstream
# of it is never created. The predict gate therefore stops the change. The
# verify gate cannot: by the time it runs the change is applied, so it fails
# the run loudly and gates whatever a later stage would do.

terraform {
  required_providers {
    forward = { source = "forwardnetworks/forward" }
    aws     = { source = "hashicorp/aws" }
  }
}

variable "network_id" { type = string }
variable "cloud_source" { type = string }
variable "route_table_id" { type = string }
variable "destination_cidr" { type = string }
variable "transit_gateway_id" { type = string }
variable "probe_src" { type = string }
variable "probe_dst" { type = string }

# 1. Baseline. A prediction is of one change against one base, so the base has
#    to be a snapshot that exists, not "whatever was latest at plan time".
resource "forward_snapshot" "baseline" {
  network_id         = var.network_id
  note               = "baseline before change"
  wait_for_processed = true
}

# 2. The same change, stated as cloud changes so it can be predicted before it
#    exists. See README: predicting the real plan file instead costs a second
#    command, because a plan cannot contain a prediction of itself.
resource "forward_predicted_snapshot" "candidate" {
  network_id       = var.network_id
  source_name      = var.cloud_source
  base_snapshot_id = forward_snapshot.baseline.id
  note             = "predict before apply"

  cloud_changes_json = jsonencode({
    routeChanges = [{
      routeTableId         = var.route_table_id
      destinationCidrBlock = var.destination_cidr
      target               = { kind = "TRANSIT_GATEWAY", id = var.transit_gateway_id }
    }]
  })
}

# 3. PREDICT GATE. Reads the network as it would be. Failing here stops the run
#    before the change is made.
data "forward_path_analysis" "predicted" {
  network_id  = var.network_id
  snapshot_id = forward_predicted_snapshot.candidate.id
  src_ip      = var.probe_src
  dst_ip      = var.probe_dst

  lifecycle {
    postcondition {
      condition     = self.forwarding_outcome == "DELIVERED"
      error_message = "Predicted ${coalesce(self.forwarding_outcome, "no path")}: refusing to apply the change."
    }
  }
}

# 4. The change itself. It depends on the gate, so a failed prediction means
#    this is never created.
resource "aws_route" "change" {
  route_table_id         = var.route_table_id
  destination_cidr_block = var.destination_cidr
  transit_gateway_id     = var.transit_gateway_id

  depends_on = [data.forward_path_analysis.predicted]
}

# 5. Collect the network as it now actually is.
resource "forward_snapshot" "after" {
  network_id         = var.network_id
  note               = "after change"
  wait_for_processed = true

  depends_on = [aws_route.change]
}

# 6. VERIFY GATE. The same question, asked of the real network. Prediction and
#    reality disagreeing is the finding worth having -- it means the model and
#    the network have diverged, whatever the change did.
data "forward_path_analysis" "actual" {
  network_id  = var.network_id
  snapshot_id = forward_snapshot.after.id
  src_ip      = var.probe_src
  dst_ip      = var.probe_dst

  lifecycle {
    postcondition {
      condition     = self.forwarding_outcome == data.forward_path_analysis.predicted.forwarding_outcome
      error_message = "Predicted ${data.forward_path_analysis.predicted.forwarding_outcome} but the collected network reports ${coalesce(self.forwarding_outcome, "no path")}."
    }
  }
}

# Intent checks are the broader verify: one path proves one thing, the check
# corpus proves the change broke nothing else anyone had written down.
data "forward_intent_checks" "after" {
  snapshot_id = forward_snapshot.after.id

  lifecycle {
    postcondition {
      condition     = self.fail_count == 0
      error_message = "${self.fail_count} intent check(s) fail on the collected network after this change."
    }
  }
}

output "baseline_snapshot" { value = forward_snapshot.baseline.id }
output "predicted_snapshot" { value = forward_predicted_snapshot.candidate.id }
output "after_snapshot" { value = forward_snapshot.after.id }
output "predicted_outcome" { value = data.forward_path_analysis.predicted.forwarding_outcome }
output "actual_outcome" { value = data.forward_path_analysis.actual.forwarding_outcome }
output "checks_failing" { value = data.forward_intent_checks.after.fail_count }
