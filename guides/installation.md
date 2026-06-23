# Installation Before Registry Publishing

This provider is not yet published to the HashiCorp Terraform Registry. Until it is, install it from a GitHub release binary or a local build.

## Local Build

```bash
go install ./...
```

Create a Terraform CLI config file:

```hcl
provider_installation {
  dev_overrides {
    "forwardnetworks/forward" = "/Users/you/go/bin"
  }
  direct {}
}
```

Run Terraform with that config:

```bash
export TF_CLI_CONFIG_FILE=/path/to/terraformrc
terraform init
terraform plan
```

## GitHub Release Binary

Download the matching archive from the GitHub release, unpack it, and place the provider binary in Terraform's local plugin tree:

```text
~/.terraform.d/plugins/registry.terraform.io/forwardnetworks/forward/0.5.0/darwin_arm64/terraform-provider-forward_v0.5.0
```

Use the platform directory that matches the workstation or runner, such as `linux_amd64`, `linux_arm64`, `darwin_amd64`, or `darwin_arm64`.

Then use the provider normally:

```hcl
terraform {
  required_providers {
    forward = {
      source  = "forwardnetworks/forward"
      version = "0.5.0"
    }
  }
}
```
