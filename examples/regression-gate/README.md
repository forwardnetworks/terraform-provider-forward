# Gating a change on the regression suite

Predict the change, evaluate the whole suite against the network as it *would*
be, and refuse to apply if the change breaks something that works today.

```sh
terraform apply \
  -var network_id=... -var device=edge-rtr-1 \
  -var commands=$'router bgp 65001\n no neighbor 10.0.0.2'
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

Predict, gate, change: it is one `terraform apply`. Terraform orders it from
the dependency graph, and the data sources read at apply time because their
snapshot ids are unknown at plan time. `terraform_data.the_change` stands in
for whatever pushes the configuration; it is created only past the gate.

**The gate fires mid-apply.** `terraform plan` cannot run it, because the
predicted snapshot does not exist yet. A blocked run still leaves the predicted
snapshot behind — that is the evidence of *why* it was blocked.
