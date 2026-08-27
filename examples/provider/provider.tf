provider "incident" {
  api_key = "<api-key>" # https://app.incident.io/settings/api-keys

  # Terraform imports resources during `plan`, and importing a resource claims it as
  # managed by Terraform. If your plans run somewhere they must not change anything -
  # on a pull request, say - turn that claim off. Resources imported this way are
  # claimed by the first apply that changes them instead.
  #
  # mark_imported_resources_as_managed = false
}
