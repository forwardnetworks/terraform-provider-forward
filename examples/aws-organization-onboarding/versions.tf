# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

terraform {
  required_version = ">= 1.0"

  required_providers {
    forward = {
      source = "forwardnetworks/forward"
    }
  }
}
