# Copyright (c) HashiCorp, Inc.

terraform {
  required_providers { forward = { source = "forwardnetworks/forward" } }
}

variable "network_id" { type = string }

# Credentials come from FORWARD_USERNAME / FORWARD_PASSWORD.
provider "forward" {
  network_id = var.network_id
}

variable "baseline_snapshot" { type = string }

# ---------------------------------------------------------------------------
# The regression suite.
#
# Every check here is persistent, which is the whole mechanism: Forward
# inherits a persistent check onto later snapshots of the same network,
# including the ones Predict produces. So the suite that guards the real
# network is the same suite that judges a change before it is made.
# ---------------------------------------------------------------------------

locals {
  # Named so the demo reads as a story rather than a list of CIDRs.
  app_tier  = "10.49.1.0/24"
  app_tier2 = "10.49.2.0/24"
  transit   = "10.50.1.0/24"
  db_tier   = "10.51.1.0/24"
  public    = "10.51.3.0/24"
  azure     = "10.48.39.0/24"
}

# --- Security posture, as NQE queries in the library ------------------------

resource "forward_nqe_library_query" "sg_open_to_internet" {
  path        = "/Demo/Security/Security groups open to the internet"
  source_code = <<-NQE
    /**
     * @intent No security group may permit ingress from the whole internet.
     */
    foreach account in network.cloudAccounts
    foreach vpc in account.vpcs
    foreach group in vpc.securityGroups
    foreach rule in group.ingressRules
    where rule.action == SecurityRuleAction.PERMIT
    foreach source in rule.match.ipv4Src
    where toString(source) == "0.0.0.0/0"
    select { account: account.name, vpc: vpc.name, group: group.name }
  NQE
}

resource "forward_nqe_library_query" "private_subnet_egress" {
  path        = "/Demo/Security/Private route tables must not reach the internet"
  source_code = <<-NQE
    /**
     * @intent A route table named private must carry no default route.
     */
    foreach account in network.cloudAccounts
    foreach vpc in account.vpcs
    foreach table in vpc.routeTables
    where isPresent(table.name)
    where matches(table.name, ".*(?i)private.*")
    foreach route in table.routes
    foreach prefix in route.prefixes
    where toString(prefix) == "0.0.0.0/0"
    select { vpc: vpc.name, table: table.name, nextHop: toString(route.nextHop) }
  NQE
}

# --- The same posture, as checks the GUI renders ----------------------------

resource "forward_intent_check" "sg_open_to_internet" {
  snapshot_id = var.baseline_snapshot
  persistent  = true
  enabled     = true
  priority    = "HIGH"
  definition_json = jsonencode({
    checkType = "NQE"
    queryId   = forward_nqe_library_query.sg_open_to_internet.query_id
  })
}

resource "forward_intent_check" "private_subnet_egress" {
  snapshot_id = var.baseline_snapshot
  persistent  = true
  enabled     = true
  priority    = "HIGH"
  definition_json = jsonencode({
    checkType = "NQE"
    queryId   = forward_nqe_library_query.private_subnet_egress.query_id
  })
}

# --- Reachability: the paths the business depends on ------------------------

locals {
  must_reach = {
    "app tier reaches the database tier"  = { from = local.app_tier, to = local.db_tier }
    "app tier reaches the transit subnet" = { from = local.app_tier, to = local.transit }
    "second app AZ reaches the database"  = { from = local.app_tier2, to = local.db_tier }
    "public tier reaches the app tier"    = { from = local.public, to = local.app_tier }
  }

  # Isolation is the security half: these must never be reachable.
  must_not_reach = {
    "database tier is not reachable from Azure"      = { from = local.azure, to = local.db_tier }
    "database tier does not reach the public subnet" = { from = local.db_tier, to = local.public }
    "public tier is isolated from the database"      = { from = local.public, to = local.db_tier }
  }
}

resource "forward_intent_check" "reachability" {
  for_each = local.must_reach

  snapshot_id = var.baseline_snapshot
  name        = each.key
  persistent  = true
  enabled     = true
  priority    = "HIGH"
  tags        = ["demo", "reachability"]
  definition_json = jsonencode({
    checkType = "Existential"
    filters = {
      from = { location = { type = "SubnetLocationFilter", value = each.value.from } }
      to   = { location = { type = "SubnetLocationFilter", value = each.value.to } }
    }
  })
}

resource "forward_intent_check" "isolation" {
  for_each = local.must_not_reach

  snapshot_id = var.baseline_snapshot
  name        = each.key
  persistent  = true
  enabled     = true
  priority    = "HIGH"
  tags        = ["demo", "security", "isolation"]
  definition_json = jsonencode({
    checkType = "Isolation"
    filters = {
      from = { location = { type = "SubnetLocationFilter", value = each.value.from } }
      to   = { location = { type = "SubnetLocationFilter", value = each.value.to } }
    }
  })
}

output "suite" {
  value = merge(
    { for k, c in forward_intent_check.reachability : k => c.status },
    { for k, c in forward_intent_check.isolation : k => c.status },
    {
      "security groups open to the internet"             = forward_intent_check.sg_open_to_internet.status
      "private route tables must not reach the internet" = forward_intent_check.private_subnet_egress.status
    },
  )
}
