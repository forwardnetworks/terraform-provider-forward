# Predicting a change before making it

`terraform plan` says what will change in the cloud. It cannot say what that does to the network. This asks Forward,
in the same run, and fails the apply if the answer is wrong.

## The gate is a postcondition, not a check block

This matters more than it looks. A failing `check` block is a **warning**: Terraform prints it and applies anyway,
exiting 0. Nothing is gated. A failing `postcondition` is an **error** -- the run stops, exits 1, and every resource
downstream of the gated data source is never created.

So the change must depend on the gate, which it does naturally: it reads the predicted snapshot.

```sh
terraform -chdir=../../path/to/your/cloud/config plan -out=change.tfplan
terraform -chdir=../../path/to/your/cloud/config show -json change.tfplan > plan.json

terraform apply -var network_id=... -var cloud_source=... -var plan_json=plan.json
```

The `forward_predicted_snapshot` resource opens a change set, stages the plan as cloud changes, commits, predicts,
and waits for the prediction to finish -- then exposes the snapshot id. Everything snapshot-scoped can be read
against it, so `forward_path_analysis` and `forward_nqe_query` answer for a network that does not exist yet.

## Why a resource and not a data source

It has effects a read must not: it creates a change set and leaves a snapshot behind. Destroying it discards the
change set. That also means it re-predicts whenever the plan, the source or the base changes -- a prediction is of
one stated change against one base, and the old answer is not an answer to the new question.

## The base snapshot

Left unset, the base is the most recent **collected** snapshot, which is not always the most recent one: every
prediction leaves a snapshot behind, and predicting from a prediction is refused.
