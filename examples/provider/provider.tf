# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

variable "forward_username" {
  description = "Forward username."
  type        = string
}

variable "forward_password" {
  description = "Forward password."
  type        = string
  sensitive   = true
}

variable "forward_base_url" {
  description = "Forward Networks API base URL."
  type        = string
}

variable "forward_network_id" {
  description = "Forward Networks network identifier."
  type        = string
}

variable "forward_insecure" {
  description = "Disable TLS certificate verification."
  type        = bool
  default     = false
}

provider "forward" {
  base_url   = var.forward_base_url
  network_id = var.forward_network_id
  username   = var.forward_username
  password   = var.forward_password
  insecure   = var.forward_insecure
}
