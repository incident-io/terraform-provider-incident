# Look up a maintenance window by name, such as one scheduled in the dashboard.
# Names aren't unique, so prefer `id` when you already have one.
data "incident_maintenance_window" "database_migration" {
  name = "Database migration"
}

# Or by ID, to reference a window another module manages.
data "incident_maintenance_window" "quiet_hours" {
  id = "01FCNDV6P870EA6S7TK1DSYDG0"
}

output "database_migration_ends_at" {
  value = data.incident_maintenance_window.database_migration.end_at
}
