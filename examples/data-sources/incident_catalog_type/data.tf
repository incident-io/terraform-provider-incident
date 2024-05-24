# Reference the catalog type by its name.
data "incident_catalog_type" "service" {
  name = "Service"
}

# Now provision the entries for the catalog type.
resource "incident_catalog_entries" "services" {
  # This uses the data source to get the ID of the catalog type. It is usually
  # adviseable that you manage your catalog types in terraform if you are also
  # managing your entries which normally means this isn't required.
  id = data.incident_catalog_type.service.id

  entries = {
    "primary" = {
      name        = "artist-web"
      description = "public-websites"
      tags        = ["java"]
    }
  }
}
