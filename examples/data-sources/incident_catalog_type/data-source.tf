# Reference the catalog type by its name.
data "incident_catalog_type" "service" {
  name = "Service"
}

# An entry's values are keyed by attribute ID, so look the attributes up by name rather
# than pinning IDs that differ between workspaces.
data "incident_catalog_type_attribute" "service_description" {
  catalog_type_id = data.incident_catalog_type.service.id
  name            = "Description"
}

data "incident_catalog_type_attribute" "service_tags" {
  catalog_type_id = data.incident_catalog_type.service.id
  name            = "Tags"
}

# Now provision the entries for the catalog type.
resource "incident_catalog_entries" "services" {
  # This uses the data source to get the ID of the catalog type. It is usually
  # adviseable that you manage your catalog types in terraform if you are also
  # managing your entries which normally means this isn't required.
  id = data.incident_catalog_type.service.id

  entries = {
    "primary" = {
      name = "artist-web"

      attribute_values = {
        # Parentheses because the key is an expression rather than a literal name.
        (data.incident_catalog_type_attribute.service_description.id) = {
          value = "public-websites"
        }
        (data.incident_catalog_type_attribute.service_tags.id) = {
          array_value = ["java"]
        }
      }
    }
  }
}
