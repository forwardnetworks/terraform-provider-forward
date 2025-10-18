# Terraform Provider Forward Enterprise

This repository contains the Terraform provider for [Forward Networks](https://www.forwardnetworks.com). The provider is built with the [Terraform Plugin Framework](https://github.com/hashicorp/terraform-plugin-framework) and uses the Forward Networks OpenAPI specification (`forward-api.json`) as the authoritative contract for future resources and data sources.

The initial pass wires up provider configuration, documentation, and a reusable API client so we can incrementally expose Forward Networks objects as Terraform resources.

## Project Status

- ✅ Provider scaffold replaced with Forward-specific configuration, environment-variable support, and HTTP client.
- ✅ Data sources implemented: platform version, network snapshots, intent checks (pass/fail summaries), and generic NQE query runner.
- ✅ Generated documentation and runnable examples kept in sync via `make generate`.
- 🚧 Next targets: expand the SDK for pagination/error handling, add managed resources (snapshot lifecycle, intent checks), and formalize release automation.

## Quick Start

```hcl
terraform {
  required_providers {
    forward = {
      source  = "forwardnetworks/forward"
      version = "~> 0.1"
    }
  }
}

variable "forward_api_key" {
  description = "Forward Networks API key."
  type        = string
  sensitive   = true
}

variable "forward_base_url" {
  description = "Forward Networks API base URL (for example, https://demo.forwardnetworks.com)."
  type        = string
}

variable "forward_network_id" {
  description = "Default Forward Enterprise network identifier."
  type        = string
}

variable "forward_insecure" {
  description = "Disable TLS verification (not recommended outside of testing)."
  type        = bool
  default     = false
}

provider "forward" {
  base_url   = var.forward_base_url
  network_id = var.forward_network_id
  api_key    = var.forward_api_key
  insecure   = var.forward_insecure
}
```

`network_id` and `base_url` must be supplied in configuration so that resources know which Forward Enterprise environment to target. Only the API key supports an environment-variable fallback natively, but Terraform variables can be populated from environment variables using the `TF_VAR_` prefix.

Example environment variable exports:

```shell
export FORWARD_API_KEY=xxxxxxxxxxxxxxxx
export TF_VAR_forward_base_url=https://demo.forwardnetworks.com
export TF_VAR_forward_network_id=123456
export TF_VAR_forward_insecure=false
```

## Requirements

- [Terraform](https://developer.hashicorp.com/terraform/downloads) >= 1.0
- [Go](https://go.dev/dl/) >= 1.24

## Building the Provider

```shell
go install ./...
```

The compiled binary is placed in `$GOBIN` (defaults to `$(go env GOPATH)/bin`).

## Developing the Provider

1. `go test ./...` to run unit tests.
2. `make generate` to refresh documentation after adding resources or data sources.
3. `make testacc` to run acceptance tests against a Forward Networks environment (these incur live API calls).

During development you can ask Terraform to load the locally-built provider by setting `TF_CLI_CONFIG_FILE` or using the global plugin cache, e.g.:

```shell
export TF_PLUGIN_CACHE_DIR="$HOME/.terraform.d/plugin-cache"
go install ./...
```

## Roadmap

1. **Authentication & Client Enhancements**  
   Finalize authentication flows (token exchange, secondary headers) and extend the SDK helper in `internal/sdk` for common request handling (pagination, error wrapping, retries).

2. **Core Resources**  
   Prioritize snapshot lifecycle, intent checks, and path analyses based on the Forward API (`forward-api.json`). Implement CRUD operations plus acceptance tests for each.

3. **Data Sources**  
   Surface read-only lookups such as inventory, intents, and compliance summaries to enable composable Terraform plans.

4. **Documentation & Examples**  
   Keep the `examples/` directory runnable and regenerate docs after each feature addition via `make generate`.

5. **Release Engineering**  
   Integrate with `goreleaser`, populate `CHANGELOG.md`, and prepare the Terraform Registry publishing metadata once the first stable resource set lands.

## Available Data Sources

- `forward_version` — exposes deployment build, release, and version metadata. [`internal/provider/version_data_source.go`](internal/provider/version_data_source.go)
- `forward_snapshots` — lists snapshots for the configured network with optional filters. [`internal/provider/snapshots_data_source.go`](internal/provider/snapshots_data_source.go)
- `forward_intent_checks` — reports intent check Pass/Fail status for a snapshot, with filterable counts. [`internal/provider/intent_checks_data_source.go`](internal/provider/intent_checks_data_source.go)
- `forward_nqe_query` — executes NQE queries and returns JSON-formatted results. [`internal/provider/nqe_query_data_source.go`](internal/provider/nqe_query_data_source.go)
