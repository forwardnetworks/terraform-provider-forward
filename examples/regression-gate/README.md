# Gating a change on the regression suite

Predict the change, evaluate the whole suite against the network as it *would*
be, and refuse to apply if the change breaks something that works today.

```sh
terraform apply \
  -var network_id=... -var cloud_source=... \
  -var route_table=rtb-... -var destination=10.92.0.0/16 \
  -var target_kind=TRANSIT_GATEWAY -var target_id=tgw-...
```

## It is a postcondition, not a check block

A failing `check` block is a **warning**. Terraform prints it and applies
anyway, exiting 0 — nothing is gated. A failing `postcondition` is an
**error**: the run stops, exits 1, and everything downstream of the gated data
source is never created.

## It asks about regression, not health

The gate compares two evaluations of the same suite — the collected baseline
and the prediction — and fails only on a check that **passes today and fails
after**.

Gating on `fail_count == 0` instead sounds stricter and is useless: real
networks always have something failing, so that gate refuses every change
forever, including the ones that fix things.

## One apply, not a pipeline

Collect, predict, gate, change, collect again, verify: it is one
`terraform apply`. Terraform orders it from the dependency graph, and the data
sources read at apply time because their snapshot ids are unknown at plan time.

Two things are worth knowing:

- **The gate fires mid-apply.** `terraform plan` cannot run it, because the
  predicted snapshot does not exist yet. A blocked run still leaves the change
  set and predicted snapshot behind — that is the evidence of *why* it was
  blocked.
- **A verify gate after the change cannot roll back.** By the time it runs, the
  change is applied. It fails the run loudly and gates whatever comes next.

## Predicting the real plan file

This example restates the change as `cloud_changes_json`, which keeps it to one
command. To predict the actual `terraform plan` output instead, use
`terraform_plan_json` — see `examples/predict-gate`. That needs two commands,
unavoidably: a plan cannot contain a prediction of itself.
