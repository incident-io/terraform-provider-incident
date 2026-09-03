# Import a schedule replica using the format schedule_id:replica_id
# Replicas belong to a schedule, so both IDs are required: use the
# ListScheduleReplicas API endpoint for a schedule to find its replicas' IDs
# Replace the IDs with real IDs from your incident.io organization
import {
  to = incident_schedule_replica.example
  id = "01ABC123DEF456GHI789JKL:01MNO456PQR789STU012VWX"
}
