# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

data "forward_aws_assume_role_external_id" "current" {}

output "forward_aws_external_id" {
  value = data.forward_aws_assume_role_external_id.current.external_id
}
