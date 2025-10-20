# Intent Check Guard Module

This module evaluates intent checks on two snapshots (typically before and
after a change) and raises Terraform pre/postcondition failures if any checks
return failing statuses.

## Inputs

- `baseline_snapshot_id` – Snapshot ID evaluated before the change.
- `verification_snapshot_id` – Snapshot ID evaluated after the change.
- `failure_statuses` – List of statuses treated as failures (default
  `FAIL`, `ERROR`, `TIMEOUT`).

## Outputs

- `baseline_summary` – Result of `forward_intent_checks` against the baseline
  snapshot.
- `verification_summary` – Result of `forward_intent_checks` against the
  verification snapshot.

## Example

```hcl
module "intent_guard" {
  source                  = "github.com/forwardnetworks/terraform-provider-forward//modules/pre-post/intent_check_guard"
  baseline_snapshot_id    = var.baseline_snapshot_id
  verification_snapshot_id = var.verification_snapshot_id
}
```
