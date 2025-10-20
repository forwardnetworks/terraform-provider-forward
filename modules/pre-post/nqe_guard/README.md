# NQE Guard Module

This module evaluates an NQE query against baseline and verification snapshots
and raises Terraform pre/postcondition failures if any rows are returned.

## Inputs

- `baseline_snapshot_id` – Snapshot ID executed before the change.
- `verification_snapshot_id` – Snapshot ID executed after the change.
- `query_id` – NQE library query identifier (optional; mutually exclusive
  with `inline_query`).
- `inline_query` – Inline NQE query source code (optional).
- `limit` – Maximum rows returned (default 100).
- `baseline_error_message` – Error message when the baseline query returns
  rows.
- `verification_error_message` – Error message when the verification query
  returns rows.

## Outputs

- `baseline_results` – Raw JSON rows from the baseline query.
- `verification_results` – Raw JSON rows from the verification query.

## Example

```hcl
module "nqe_guard" {
  source                   = "github.com/forwardnetworks/terraform-provider-forward//modules/pre-post/nqe_guard"
  baseline_snapshot_id     = var.baseline_snapshot_id
  verification_snapshot_id = var.verification_snapshot_id
  query_id                 = forward_nqe_query_definition.drift.query_id
}
```
