# Pre/Post Change Validation Example

This example demonstrates how to use the Forward Terraform provider to gate a
change window with pre/post validation. It assumes that you capture a baseline
snapshot before making any network modifications and a verification snapshot
afterwards.

The workflow illustrates:

1. Creating (or reasserting) an intent check tied to the baseline snapshot.
2. Running the `forward_intent_checks` data source to confirm no failures before
   the change and against the verification snapshot afterwards.
3. Executing a stored NQE query for both snapshots and rejecting the plan/apply
   if either returns rows.

## Usage

Populate the following environment variables (or provide values via
`terraform.tfvars`):

```shell
export TF_VAR_forward_api_key="..."
export TF_VAR_forward_network_id="..."
export TF_VAR_forward_base_url="https://demo.forwardnetworks.com"
export TF_VAR_baseline_snapshot_id="..."
export TF_VAR_verification_snapshot_id="..."
export TF_VAR_intent_check_definition="$(cat intent_check.json)"
export TF_VAR_nqe_query_path="/L3/MtuConsistency"
```

The intent check definition must be valid JSON matching the Forward Enterprise
`NewNetworkCheck` schema. For example, an NQE-based check could look like:

```json
{
  "checkType": "NQE",
  "queryId": "FQ_example",
  "params": {}
}
```

After setting the variables run:

```shell
terraform init
terraform plan
```

The `null_resource` guards will raise precondition or postcondition errors if
intent checks or NQE results indicate issues before or after the change.
