# Reference the incident schedule by its id.
data "incident_schedule" "by_id" {
  id = "01HPFH8T92MPGSQS5C1SPAF4V0"
}

# Reference the incident schedule by its name (case sensitive).
data "incident_schedule" "by_name" {
  name = "Primary On-call"
}

# Output the schedule details
output "schedule_id" {
  value = data.incident_schedule.by_name.id
}

output "schedule_timezone" {
  value = data.incident_schedule.by_name.timezone
}

output "schedule_team_ids" {
  value = data.incident_schedule.by_name.team_ids
}

