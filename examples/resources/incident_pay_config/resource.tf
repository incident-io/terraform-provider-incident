# A pay config sets what someone is paid for being on call: a base rate, plus rules that
# override it at particular times. Every rate is in the lowest denomination of the
# currency, per rate_time_unit, and pay is pro-rated by the second either way.
resource "incident_pay_config" "platform" {
  name            = "Platform on-call"
  timezone        = "Europe/London"
  currency        = "GBP"
  base_rate_cents = 500 # £5.00 per hour, for any time no rule covers
  rate_time_unit  = "hour"

  # Weekly rules are evaluated in order, and the first one that covers a shift prices
  # it. Put the rule that should win first: here, weekend nights fall under the weekend
  # rule rather than the night rule.
  #
  # A rule runs from start_time to end_time within one day, so a weeknight is two rules:
  # the evening, and the following morning. 00:00 as an end_time is midnight at the end
  # of the day.
  weekly_rules = [
    {
      weekdays   = ["saturday", "sunday"]
      start_time = "00:00"
      end_time   = "00:00" # 00:00 to 00:00 is the whole day
      rate_cents = 1500
    },
    {
      weekdays   = ["monday", "tuesday", "wednesday", "thursday", "friday"]
      start_time = "18:00"
      end_time   = "00:00" # the evening, up to midnight
      rate_cents = 1000
    },
    {
      weekdays   = ["monday", "tuesday", "wednesday", "thursday", "friday"]
      start_time = "00:00"
      end_time   = "09:00" # the morning after, at the same rate
      rate_cents = 1000
    },
  ]

  # One-off rules take precedence over the weekly rules and may not overlap each other.
  one_off_rules = [
    {
      name       = "Christmas Day"
      start_at   = "2026-12-25T00:00:00Z"
      end_at     = "2026-12-26T00:00:00Z"
      rate_cents = 3000
    },
    {
      name       = "Boxing Day"
      start_at   = "2026-12-26T00:00:00Z"
      end_at     = "2026-12-27T00:00:00Z"
      rate_cents = 2500
    },
  ]
}

# A flat rate needs no rules at all.
resource "incident_pay_config" "flat_daily" {
  name            = "Flat daily rate"
  timezone        = "America/New_York"
  currency        = "USD"
  base_rate_cents = 10000 # $100.00 per day
  rate_time_unit  = "day"
}
