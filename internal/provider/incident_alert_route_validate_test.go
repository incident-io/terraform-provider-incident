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
// exercise the mode-gating and enabled-gating rules without a live org.

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
	arMessageDisabled = `
  message_config = {
    enabled = false
  }`
	arMessageDisabledWithTemplate = `
  message_config = {
    enabled = false
    template = {
      value = {
        literal = "01TMPL"
      }
    }
  }`
	arEscalationDisabled = `
  escalation_config = {
    enabled = false
  }`
	arEscalationDisabledWithCancel = `
  escalation_config = {
    enabled                 = false
    auto_cancel_escalations = true
  }`
	arEscalationEnabledNoCancel = `
  escalation_config = {
    enabled            = true
    escalation_targets = []
  }`
	arEscalationEnabledWithJoins = `
  escalation_config = {
    enabled                 = true
    auto_cancel_escalations = true
    escalation_targets      = []
    when_alert_joins_group = {
      mode                 = "on_each_new_alert"
      grace_period_seconds = 60
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

func TestIncidentAlertRouteResource_ValidateConfig(t *testing.T) {
	cases := []struct {
		name   string
		config string
		errRe  string // empty => expect a successful (non-empty) plan
	}{
		{
			name:   "v3 valid minimal",
			config: arValidateConfig(arGroupingDisabled + arMessageDisabled + arEscalationDisabled + arIncidentEnabled()),
		},
		{
			name:   "v3 forbids channel_config",
			config: arValidateConfig(arGroupingDisabled + arMessageDisabled + arEscalationDisabled + arIncidentEnabled() + arChannelConfig),
			errRe:  "channel_config` belongs to the v2 schema",
		},
		{
			name:   "v3 escalation enabled requires auto_cancel_escalations",
			config: arValidateConfig(arGroupingDisabled + arMessageDisabled + arEscalationEnabledNoCancel + arIncidentEnabled()),
			errRe:  "auto_cancel_escalations` is required when",
		},
		{
			name:   "v3 escalation disabled forbids auto_cancel_escalations",
			config: arValidateConfig(arGroupingDisabled + arMessageDisabled + arEscalationDisabledWithCancel + arIncidentEnabled()),
			errRe:  "must not be set when `escalation_config.enabled` is false",
		},
		{
			name:   "v3 message disabled forbids template",
			config: arValidateConfig(arGroupingDisabled + arMessageDisabledWithTemplate + arEscalationDisabled + arIncidentEnabled()),
			errRe:  "must not be set when `message_config.enabled` is false",
		},
		{
			name:   "v3 when_alert_joins_group requires grouping enabled",
			config: arValidateConfig(arGroupingDisabled + arMessageDisabled + arEscalationEnabledWithJoins + arIncidentEnabled()),
			errRe:  "only valid when `grouping_config.default.enabled` is true",
		},
		{
			name:   "v3 grouping enabled requires window_seconds",
			config: arValidateConfig(arGroupingEnabledNoWindow + arMessageDisabled + arEscalationDisabled + arIncidentEnabled()),
			errRe:  "window_seconds` is required when",
		},
		{
			name:   "v3 incident disabled forbids template",
			config: arValidateConfig(arGroupingDisabled + arMessageDisabled + arEscalationDisabled + arIncidentDisabledWithTemplate()),
			errRe:  "`incident_config.template` must not be set when",
		},
		{
			name:   "v2 valid minimal",
			config: arV2Config("", arV2IncidentTemplate),
		},
		{
			name:   "v2 forbids message_config",
			config: arV2Config(arMessageDisabled, arV2IncidentTemplate),
			errRe:  "message_config` belongs to the v3 schema",
		},
		{
			name:   "v2 requires incident_template",
			config: arV2Config("", ""),
			errRe:  "`incident_template` is required in the v2 schema",
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
