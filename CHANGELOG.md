## 0.7.0 (October 13, 2026)

FEATURES:
- `forward_predicted_snapshot` takes `device_commands`: a map of Forward device name to the configuration-mode lines to stage on it, on the same change set as any `firewall_rules`, so a change touching routers and firewalls is one prediction.
- New `forward_collector` resource registers a collector with Forward and returns its `token` (`username:password`, the Docker collector's `TOKEN`), so a collector deployment needs no Forward UI step. Forward shows the token only at registration: it is kept in state, survives refresh, and is null after an import. Destroying the resource deletes the collector in Forward. Tokens in Forward's newer (V3) format are decoded to the same `username:password` form, with the server they were issued for in `app_server_url`.

BREAKING CHANGES:
- Cloud Predict is removed from `forward_predicted_snapshot`: `source_name`, `terraform_plan_json`, `route_changes`, `imported_objects`, `unsupported_resources` and `staged_changes_json` are gone, with `examples/predict-gate` and `examples/full-lifecycle`. The cloud-object change-set routes they called do not exist on any released Forward build. Predictions now come from `device_commands` and `firewall_rules`; `examples/regression-gate` uses `device_commands`.
- The provider builds against `forward-go-sdk` v0.4.12.

## 0.6.0 (September 18, 2026)

FEATURES:
- Cloud Predict: `forward_predicted_snapshot` predicts what a Terraform plan does to the network before it is applied, and `forward_snapshot` collects the estate afterwards to verify it. With `precondition`/`postcondition` on an intent check, an apply is gated on the prediction (see `examples/predict-gate`).

BREAKING CHANGES:
- `forward_predicted_snapshot` now stages changes through Forward's cloud-object change-set routes. `cloud_changes_json` is gone; state a change directly with the new `route_changes` list (one entry per route: `route_table_id`, `destination`, optional `target` — omit it to remove the route) or keep passing `terraform_plan_json`.
- The provider now builds against the public `forward-go-sdk` module; `CloudAccountRequest` fields are plain values rather than pointers.

IMPROVEMENTS:
- `forward_predicted_snapshot` exposes `staged_changes_json`: each imported object's diff as Forward models it, for a page or report to render without a second client.
- `forward_predicted_snapshot` exposes `imported_objects` (the route tables and security groups the plan touched) and `unsupported_resources` (plan resources Forward has no predictor for), and warns when a plan is only partly modelled instead of predicting it as a smaller change.

## 0.5.1

IMPROVEMENTS:
- `forward_aws_cloud_account` now adopts an existing Forward AWS setup with the same name and patches it during first apply, matching the update behavior used by later Terraform runs.
- Added `allow_account_removals`, defaulting to `false`, so Terraform refuses to remove AWS account entries from an existing Forward setup unless the operator explicitly confirms the removal.

## 0.5.0

FEATURES:
- Added Terraform-native AWS Organizations onboarding with `forward_aws_assume_role_external_id`, `forward_aws_organization_accounts`, and `forward_aws_cloud_account`.
- Added three AWS collection credential models for Forward setups: Forward assume-role, static collector keys, and collector instance profile.
- Added core SDK support for snapshot lifecycle, path analysis, intent checks, and NQE query execution.
- Implemented Terraform resources `forward_intent_check`, `forward_nqe_query_definition`, and `forward_snapshot`.
- Added data sources `forward_snapshots`, `forward_intent_checks`, `forward_nqe_query`, `forward_path_analysis`, and `forward_version`.
- Published reusable modules for pre/post change validation combining intent checks and NQE queries.

NOTES:
- Forward authentication is Basic auth only.
- Static-key AWS collection sends collector key material to Forward and stores sensitive values in Terraform state.
