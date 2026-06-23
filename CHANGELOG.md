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
