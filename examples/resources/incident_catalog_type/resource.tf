# Create a catalog type for a service tier, representing how important a service is.
resource "incident_catalog_type" "service_tier" {
  name            = "ServiceTier"
  type_name       = "Custom[\"ServiceTier\"]"
  description     = <<EOF
  How critical is this service, with tier 1 being the highest and 3 the lowest.
  EOF
  categories      = ["service"]
  source_repo_url = "https://github.com/mycompany/infrastructure"
}
