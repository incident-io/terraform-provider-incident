package provider

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// wrapRe builds a matcher for an expected diagnostic phrase that tolerates the
// line-wrapping Terraform applies to diagnostic text (spaces may become
// newlines + indentation). It joins the phrase's words with \s+.
func wrapRe(phrase string) *regexp.Regexp {
	return regexp.MustCompile(strings.Join(strings.Fields(phrase), `\s+`))
}

// These are hermetic unit tests (IsUnitTest: true) for the alert route
// ValidateConfig logic: they run a plan-only step with literal IDs, so they
// exercise the mode-gating and conditional rules without a live org.

// arValidateConfig assembles a full incident_alert_route config from the
// variable blocks (everything after the common scaffolding). Using literal IDs
// keeps the plan hermetic.
func arValidateConfig(blocks string) string {
	return fmt.Sprintf(`
resource "incident_alert_route" "test" {
  name       = "validate-test"
  enabled    = true
  is_private = false

  alert_sources = [
    {
      alert_source_id  = "01SRC"
      condition_groups = []
    }
  ]
  condition_groups = []
  expressions      = []
%s
}
`, blocks)
}

const (
	arGroupingDisabled = `
  grouping_config = {
    default = {
      enabled = false
    }
  }`
	arGroupingEnabledNoWindow = `
  grouping_config = {
    default = {
      enabled       = true
      grouping_keys = []
    }
  }`
	arMessageValid = `
  message_config = {
    destinations = []
  }`
	arEscalationValid = `
  escalation_config = {
    auto_cancel_escalations = true
    escalation_targets      = []
  }`
	arEscalationPriorityGrace = `
  escalation_config = {
    auto_cancel_escalations = true
    escalation_targets      = []
    when_alert_joins_group = {
      mode                 = "on_priority_increase"
      grace_period_seconds = 60
    }
  }`
	arEscalationJoinsGroup = `
  escalation_config = {
    auto_cancel_escalations = true
    escalation_targets      = []
    when_alert_joins_group = {
      mode = "on_each_new_alert"
    }
  }`
	arChannelConfig = `
  channel_config = [
    {
      condition_groups = []
      slack_targets = {
        channel_visibility = "public"
        binding = {
          array_value = [
            {
              literal = "C123"
            }
          ]
        }
      }
    }
  ]`
)

// arIncidentEnabled is a valid v3 incident_config with the template nested.
func arIncidentEnabled() string {
	return `
  incident_config = {
    auto_decline_enabled = true
    enabled              = true
    condition_groups     = []
` + alertRouteV3IncidentTemplateBlock + `
  }`
}

// arIncidentDisabledWithTemplate sets a template while incident creation is off.
func arIncidentDisabledWithTemplate() string {
	return `
  incident_config = {
    enabled          = false
    condition_groups = []
` + alertRouteV3IncidentTemplateBlock + `
  }`
}

// arV2Config is a minimal valid v2-mode config (no grouping_config). The
// optional mutate blocks let cases add/remove pieces.
func arV2Config(extra, incidentTemplate string) string {
	return fmt.Sprintf(`
resource "incident_alert_route" "test" {
  name       = "validate-test"
  enabled    = true
  is_private = false

  alert_sources = [
    {
      alert_source_id  = "01SRC"
      condition_groups = []
    }
  ]
  condition_groups = []
  expressions      = []

  escalation_config = {
    auto_cancel_escalations = true
    escalation_targets      = []
  }

  incident_config = {
    auto_decline_enabled    = false
    enabled                 = false
    condition_groups        = []
    grouping_keys           = []
    grouping_window_seconds = 300
    defer_time_seconds      = 0
  }
%[1]s
%[2]s
}
`, incidentTemplate, extra)
}

const arV2IncidentTemplate = `
  incident_template = {
    name    = {}
    summary = {}
  }`

// TestIncidentAlertRouteResource_ValidateConfigUnknownSkipsRequired guards the
// M1 fix: a "required" attribute whose value is unknown at plan time (here
// grouping_config.default.window_seconds, derived from a not-yet-created
// incident_alert_source's computed id) must NOT produce a "Missing required
// attribute" error, because it may well be present at apply.
//
// The unknown is sourced from an incident_alert_source rather than
// terraform_data so the test runs on Terraform 1.2 (terraform_data is 1.4+).
func TestIncidentAlertRouteResource_ValidateConfigUnknownSkipsRequired(t *testing.T) {
	config := `
resource "incident_alert_source" "trigger" {
  name        = "validate-unknown-source"
  source_type = "http"
  template = {
    title       = { literal = "t" }
    description = { literal = "d" }
    attributes  = []
    expressions = []
  }
}

resource "incident_alert_route" "test" {
  name       = "validate-unknown"
  enabled    = true
  is_private = false

  alert_sources = [
    {
      alert_source_id  = "01SRC"
      condition_groups = []
    }
  ]
  condition_groups = []
  expressions      = []

  grouping_config = {
    default = {
      enabled     = true
      window_type = "rolling"
      # Unknown at plan time (derived from a not-yet-created resource's computed
      # id): must not trip the "window_seconds is required" check.
      window_seconds = length(incident_alert_source.trigger.id) > 0 ? 300 : 600
    }
  }

  message_config = {
    destinations = []
  }

  escalation_config = {
    auto_cancel_escalations = true
    escalation_targets      = []
  }

  incident_config = {
    auto_decline_enabled = true
    enabled              = true
    condition_groups     = []
` + alertRouteV3IncidentTemplateBlock + `
  }
}
`

	resource.Test(t, resource.TestCase{
		IsUnitTest:               true,
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:             config,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

func TestIncidentAlertRouteResource_ValidateConfig(t *testing.T) {
	cases := []struct {
		name   string
		config string
		errRe  string // empty => expect a successful (non-empty) plan
	}{
		{
			name:   "v3 valid minimal",
			config: arValidateConfig(arGroupingDisabled + arMessageValid + arEscalationValid + arIncidentEnabled()),
		},
		{
			name:   "v3 forbids channel_config",
			config: arValidateConfig(arGroupingDisabled + arMessageValid + arEscalationValid + arIncidentEnabled() + arChannelConfig),
			errRe:  "channel_config` cannot be used with the v3 alert route schema",
		},
		{
			name:   "v3 grouping enabled requires window_seconds",
			config: arValidateConfig(arGroupingEnabledNoWindow + arMessageValid + arEscalationValid + arIncidentEnabled()),
			errRe:  "window_seconds` is required when",
		},
		{
			name:   "v3 incident disabled forbids template",
			config: arValidateConfig(arGroupingDisabled + arMessageValid + arEscalationValid + arIncidentDisabledWithTemplate()),
			errRe:  "`incident_config.template` must not be set when",
		},
		{
			name:   "v3 grace_period only for on_each_new_alert",
			config: arValidateConfig(arGroupingDisabled + arMessageValid + arEscalationPriorityGrace + arIncidentEnabled()),
			errRe:  "grace_period_seconds` can only be set when",
		},
		{
			name:   "v3 when_alert_joins_group requires grouping enabled",
			config: arValidateConfig(arGroupingDisabled + arMessageValid + arEscalationJoinsGroup + arIncidentEnabled()),
			errRe:  "when_alert_joins_group` can only be set when",
		},
		{
			name:   "v2 valid minimal",
			config: arV2Config("", arV2IncidentTemplate),
		},
		{
			name:   "v2 forbids message_config",
			config: arV2Config(arMessageValid, arV2IncidentTemplate),
			errRe:  "message_config` belongs to the v3 schema",
		},
		{
			name:   "v2 requires incident_template",
			config: arV2Config("", ""),
			errRe:  "`incident_template` is required when using the v2 alert route schema",
		},
		{
			name: "v2 requires auto_decline_enabled",
			config: `
resource "incident_alert_route" "test" {
  name       = "validate-test"
  enabled    = true
  is_private = false

  alert_sources = [
    {
      alert_source_id  = "01SRC"
      condition_groups = []
    }
  ]
  condition_groups = []
  expressions      = []

  escalation_config = {
    auto_cancel_escalations = true
    escalation_targets      = []
  }

  incident_config = {
    enabled                 = false
    condition_groups        = []
    grouping_keys           = []
    grouping_window_seconds = 300
    defer_time_seconds      = 0
  }
` + arV2IncidentTemplate + `
}
`,
			errRe: "auto_decline_enabled` is required when using the v2 alert route schema",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			step := resource.TestStep{
				Config:   tc.config,
				PlanOnly: true,
			}
			if tc.errRe == "" {
				step.ExpectNonEmptyPlan = true
			} else {
				step.ExpectError = wrapRe(tc.errRe)
			}

			resource.Test(t, resource.TestCase{
				IsUnitTest:               true,
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps:                    []resource.TestStep{step},
			})
		})
	}
}
