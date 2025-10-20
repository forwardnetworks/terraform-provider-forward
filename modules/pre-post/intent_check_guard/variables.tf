# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

# Copyright (c) HashiCorp, Inc.

variable "baseline_snapshot_id" {
  description = "Snapshot identifier captured before the change."
  type        = string
}

variable "verification_snapshot_id" {
  description = "Snapshot identifier captured after the change."
  type        = string
}

variable "failure_statuses" {
  description = "Intent check statuses that should be treated as failures."
  type        = list(string)
  default     = ["FAIL", "ERROR", "TIMEOUT"]
}
