# ===========================================================================
# business_services.tf — business services + dependency edges.
# ---------------------------------------------------------------------------
# Two business services per admin (customer-checkout, product-discovery),
# each wired via service-dependency edges to the technical services that
# support it — same shape as single-user, namespaced per admin.
# ===========================================================================

resource "pagerduty_business_service" "biz" {
  for_each = local.admin_biz

  name             = "${each.value.admin}-${each.value.service}"
  description      = local.business_services[each.value.service].description
  point_of_contact = "${each.value.admin}-SRE-TEAM"
  team             = pagerduty_team.admin[each.value.admin].id
}

resource "pagerduty_service_dependency" "edge" {
  for_each = local.admin_dependency_edges

  dependency {
    dependent_service {
      id   = pagerduty_business_service.biz["${each.value.admin}/${each.value.biz}"].id
      type = "business_service"
    }
    supporting_service {
      id   = pagerduty_service.tech["${each.value.admin}/${each.value.tech}"].id
      type = "service"
    }
  }
}
