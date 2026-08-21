# The regression suite

Intent checks and NQE queries, declared as Terraform. They land in Forward and
show up in the GUI like any other check — which is the point: the suite a
person browses is the suite the pipeline gates on.

```sh
terraform apply -var network_id=... -var baseline_snapshot=...
```

## Why `persistent = true` matters

A persistent check is inherited by every later snapshot of the same network,
**including the ones Predict produces**. That single property is what turns a
check corpus into a pre-change regression suite: nothing needs re-running
against the prediction, because Forward already evaluated it there.

Drop `persistent`, and a check only ever describes the one snapshot it was
created on.

## What is in it

| Kind | Asks |
|---|---|
| `Existential` | a path that must exist — app to database, public to app |
| `Isolation` | a path that must **not** exist — the security half |
| `NQE` | posture over the cloud model: security groups open to `0.0.0.0/0`, private route tables with a default route |

The NQE checks are backed by `forward_nqe_library_query`, which publishes the
query to the organization's library. That is a two-stage flow inside Forward —
a draft in your workspace, then a commit — and both stages matter: an
uncommitted draft is invisible to everyone else and cannot back a check.

## Naming

The check name is what the gate reports when something breaks, so these are
written as sentences: "public tier is isolated from the database" reads better
in a failed pipeline than a pair of CIDRs.

An NQE check is the exception — Forward names it after its query and rejects
the field outright, which is why `name` is left unset for those.
