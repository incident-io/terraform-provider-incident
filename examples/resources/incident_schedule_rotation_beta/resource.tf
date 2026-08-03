# A rotation is a group of people who take turns being on call. Each one is managed
# on its own, so editing this rotation doesn't touch the others on the schedule.
resource "incident_schedule_rotation_beta" "primary" {
  schedule_id = incident_schedule_beta.platform.id
  name        = "Primary"

  # The people in the rotation, in the order they take shifts.
  users = [
    data.incident_user.alice.id,
    data.incident_user.bob.id,
  ]

  # Hand over every Monday at 09:00, counting from this moment.
  first_interval_starts_at = "2024-01-08T09:00:00Z"
  handovers = [
    {
      interval      = 1
      interval_type = "weekly"
    }
  ]

  # How many people are on call at the same time.
  concurrent_shifts = 1
}

# A rotation that only covers weekday working hours, with two people on call at
# once and a place in the schedule's running order.
resource "incident_schedule_rotation_beta" "business_hours" {
  schedule_id = incident_schedule_beta.platform.id
  name        = "Business hours"
  rank        = 2

  users = [
    data.incident_user.alice.id,
    data.incident_user.bob.id,
    data.incident_user.carol.id,
  ]

  first_interval_starts_at = "2024-01-08T09:00:00Z"
  handovers = [
    {
      interval      = 1
      interval_type = "daily"
    }
  ]

  concurrent_shifts = 2

  # Omit this to keep the rotation on call around the clock. An empty list isn't a
  # way to say that.
  working_intervals = [
    { weekday = "monday", start_time = "09:00", end_time = "17:00" },
    { weekday = "tuesday", start_time = "09:00", end_time = "17:00" },
    { weekday = "wednesday", start_time = "09:00", end_time = "17:00" },
    { weekday = "thursday", start_time = "09:00", end_time = "17:00" },
    { weekday = "friday", start_time = "09:00", end_time = "17:00" },
  ]
}
