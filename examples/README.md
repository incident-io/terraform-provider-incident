# Examples

This directory contains examples that are mostly used for documentation, but can
also be run/tested manually via the Terraform CLI.

The document generation tool looks for files in the following locations by
default. All other *.tf files besides the ones mentioned below are ignored by
the documentation tool. This is useful for creating examples that can run and/or
ar testable even if some parts are not relevant for the documentation.

* **provider/provider.tf** example file for the provider index page
* **data-sources/`full data source name`/data-source.tf** example file for the
  named data source page
* **resources/`full resource name`/resource.tf** example file for the named data
  source page

## Multiple examples for one page

A resource can have more than one example, as **resource-`example
name`.tf** files instead of a single resource.tf: the documentation tool picks
up everything matching resource*.tf, in filename order.

By default they're all rendered into one "Example Usage" section. To give each
one its own titled section, as incident_workflow does, add a template for the
page in templates/resources/`name without the incident_ prefix`.md.tmpl and
render each file with `tffile`. Bear in mind that a template lists its examples
explicitly, so a new example file needs a section adding there too, or it won't
appear in the docs.
