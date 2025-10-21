# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

# Copyright (c) HashiCorp, Inc.

variable "forward_api_key" {
  description = "Forward Networks API key (use TF_VAR_forward_api_key or FORWARD_API_KEY)."
  type        = string
  sensitive   = true
}

variable "forward_base_url" {
  description = "Forward Networks API base URL."
  type        = string
  default     = "https://fwd.app"
}

variable "forward_network_id" {
  description = "Forward Enterprise network identifier."
  type        = string
}

variable "forward_insecure" {
  description = "Disable TLS verification (not recommended)."
  type        = bool
  default     = false
}

variable "baseline_snapshot_id" {
  description = "Snapshot identifier captured before the change."
  type        = string
}

variable "verification_snapshot_id" {
  description = "Snapshot identifier captured after the change."
  type        = string
}

variable "intent_check_name" {
  description = "Human readable name for the baseline intent check."
  type        = string
  default     = "change-window-intent"
}

variable "intent_check_definition" {
  description = "JSON string payload describing the intent check definition (matches Forward API schema)."
  type        = string
}

variable "nqe_query_path" {
  description = "Library path of the stored NQE query used for validation (e.g. /L3/MtuConsistency)."
  type        = string
}

variable "nqe_repository" {
  description = "Repository containing the NQE query (ORG or FWD)."
  type        = string
  default     = "ORG"
}

variable "nqe_query_limit" {
  description = "Maximum number of rows to retrieve when executing the NQE query."
  type        = number
  default     = 100
}
