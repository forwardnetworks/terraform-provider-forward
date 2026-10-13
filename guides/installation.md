# Installation Before Registry Publishing

This provider is not yet published to the HashiCorp Terraform Registry, so
`terraform init` cannot download it. Until it is, install a GitHub release into
Terraform's local plugin directory, or use a local build.

## GitHub Release

Once per machine (Linux or macOS). The checksum is checked before anything is
installed:

```sh
VERSION=0.7.1
OS=$(uname -s | tr '[:upper:]' '[:lower:]')                  # linux or darwin
ARCH=$(uname -m | sed 's/x86_64/amd64/; s/aarch64/arm64/')   # amd64 or arm64
URL=https://github.com/forwardnetworks/terraform-provider-forward/releases/download/v$VERSION
ZIP=terraform-provider-forward_${VERSION}_${OS}_${ARCH}.zip
DIR=~/.terraform.d/plugins/registry.terraform.io/forwardnetworks/forward/$VERSION/${OS}_${ARCH}

cd "$(mktemp -d)"
curl -fsSLO "$URL/$ZIP"
curl -fsSLO "$URL/terraform-provider-forward_${VERSION}_SHA256SUMS"
grep " $ZIP\$" terraform-provider-forward_${VERSION}_SHA256SUMS | shasum -a 256 -c -
mkdir -p "$DIR" && unzip -o "$ZIP" -d "$DIR"
```

On Windows, unzip `terraform-provider-forward_<version>_windows_amd64.zip` into
`%APPDATA%\terraform.d\plugins\registry.terraform.io\forwardnetworks\forward\<version>\windows_amd64\`.

Then use the provider normally:

```hcl
terraform {
  required_providers {
    forward = {
      source  = "forwardnetworks/forward"
      version = ">= 0.7.1"
    }
  }
}
```

`terraform init` reports `Installed forwardnetworks/forward v0.7.1
(unauthenticated)`: Terraform does not check signatures for providers installed
this way, which is why the commands above check the release's SHA-256 sum.
To check the signature as well, import the release's
`terraform-provider-forward_signing_key.asc` and run
`gpg --verify terraform-provider-forward_<version>_SHA256SUMS.sig terraform-provider-forward_<version>_SHA256SUMS`.

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

Run Terraform with that config (`terraform init` warns that development
overrides are in effect; that is expected):

```bash
export TF_CLI_CONFIG_FILE=/path/to/terraformrc
terraform init
terraform plan
```
