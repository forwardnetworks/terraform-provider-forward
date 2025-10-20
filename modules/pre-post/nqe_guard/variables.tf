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

variable "query_id" {
  description = "Stored NQE library query identifier."
  type        = string
  default     = ""
}

variable "inline_query" {
  description = "Inline NQE query. Only used when query_id is empty."
  type        = string
  default     = ""
}

variable "limit" {
  description = "Maximum number of rows returned by the NQE query."
  type        = number
  default     = 100
}

variable "baseline_error_message" {
  description = "Error message when baseline results are non-empty."
  type        = string
  default     = "Baseline NQE query returned unexpected results."
}

variable "verification_error_message" {
  description = "Error message when verification results are non-empty."
  type        = string
  default     = "Verification NQE query returned results indicating drift."
}

