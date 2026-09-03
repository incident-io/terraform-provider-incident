package provider

import (
	"fmt"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/v6/internal/client"
)

func accIncidentScheduleResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and read
			{
				Config: testAccIncidentScheduleResourceConfig(&client.ScheduleV2{
					Name:     StableSuffix("example"),
					Timezone: "Europe/London",
					Config: &client.ScheduleConfigV2{
						Rotations: []client.ScheduleRotationV2{
							{
								Id:              "rota-primary",
								HandoverStartAt: time.Date(2024, 4, 26, 16, 0, 0, 0, time.UTC),
								Name:            "Rota",
								Handovers: []client.ScheduleRotationHandoverV2{
									{
										IntervalType: client.ScheduleRotationHandoverV2IntervalType("weekly"),
										Interval:     1,
									},
								},
								Layers: []client.ScheduleLayerV2{
									{
										Id:   lo.ToPtr("rota-primary-layer-one"),
										Name: lo.ToPtr("Primary Layer One"),
									},
								},
							},
							{
								Id:              "rota-primary",
								HandoverStartAt: time.Date(2024, 4, 26, 16, 0, 0, 0, time.UTC),
								EffectiveFrom:   lo.ToPtr(time.Now().Add(time.Hour * 24).UTC()),
								Name:            "Rota",
								Handovers: []client.ScheduleRotationHandoverV2{
									{
										IntervalType: client.ScheduleRotationHandoverV2IntervalType("weekly"),
										Interval:     1,
									},
								},
								Layers: []client.ScheduleLayerV2{
									{
										Id:   lo.ToPtr("rota-primary-layer-one"),
										Name: lo.ToPtr("Primary Layer One"),
									},
								},
								WorkingInterval: &[]client.ScheduleRotationWorkingIntervalV2{
									{
										StartTime: "09:00",
										EndTime:   "17:00",
										Weekday:   "monday",
									},
								},
							},
						},
					},
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_schedule.example", "name", StableSuffix("example"),
					),
					resource.TestCheckResourceAttr(
						"incident_schedule.example", "timezone", "Europe/London",
					),
					resource.TestCheckResourceAttr(
						"incident_schedule.example", "holidays_public_config.country_codes.0", "GB",
					),
					resource.TestCheckResourceAttr(
						"incident_schedule.example", "holidays_public_config.country_codes.1", "FR",
					),
				),
			},
			// Import
			{
				ResourceName:      "incident_schedule.example",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func accIncidentScheduleResourceTimezoneUpdate(t *testing.T) {

	var scheduleID *string

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Initial creation
			{
				Config: testAccIncidentScheduleResourceConfig(&client.ScheduleV2{
					Name:     StableSuffix("timezone-test"),
					Timezone: "Europe/London",
				}),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_schedule.example", "timezone", "Europe/London",
					),
					// Store the current id
					resource.TestCheckResourceAttrWith("incident_schedule.example", "id", func(id string) (err error) {
						scheduleID = &id
						return nil
					}),
				),
			},
			// Attempt to update timezone - should destroy the existing and recreate
			{
				Config: testAccIncidentScheduleResourceConfig(&client.ScheduleV2{
					Name:     StableSuffix("timezone-test"),
					Timezone: "America/New_York",
				}),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet("incident_schedule.example", "id"), // Ensure resource exists
					// Check that the updated resource is replaced
					resource.TestCheckResourceAttrWith("incident_schedule.example", "id", func(id string) (err error) {
						if *scheduleID == id {
							return fmt.Errorf("expected new resource to be created as timezone has RequiresReplace")
						}
						return nil
					}),
				),
			},
		},
	})
}

func accIncidentScheduleResourceRotationUpdates(t *testing.T) {
	var (
		effectiveFrom   = time.Now().Add(24 * time.Hour).UTC()
		handoverStartAt = time.Date(2024, 4, 26, 16, 0, 0, 0, time.UTC)
	)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create with initial rotation
			{
				Config: testAccIncidentScheduleResourceConfig(&client.ScheduleV2{
					Name:     StableSuffix("rotation-test"),
					Timezone: "Europe/London",
					Config: &client.ScheduleConfigV2{
						Rotations: []client.ScheduleRotationV2{
							{
								Id:              "rota-test",
								Name:            "Test Rota",
								HandoverStartAt: handoverStartAt,
								Handovers: []client.ScheduleRotationHandoverV2{
									{
										Interval:     1,
										IntervalType: client.ScheduleRotationHandoverV2IntervalTypeWeekly,
									},
								},
								Layers: []client.ScheduleLayerV2{
									{
										Id:   lo.ToPtr("layer-1"),
										Name: lo.ToPtr("Layer One"),
									},
								},
							},
						},
					},
				}),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_schedule.example", "rotations.0.name", "Test Rota",
					),
					resource.TestCheckResourceAttr(
						"incident_schedule.example", "rotations.0.versions.0.layers.0.name", "Layer One",
					),
				),
			},
			// Add a new version to the rotation
			{
				Config: testAccIncidentScheduleResourceConfig(&client.ScheduleV2{
					Name:     StableSuffix("rotation-test"),
					Timezone: "Europe/London",
					Config: &client.ScheduleConfigV2{
						Rotations: []client.ScheduleRotationV2{
							{
								Id:              "rota-test",
								Name:            "Test Rota",
								HandoverStartAt: handoverStartAt,
								Handovers: []client.ScheduleRotationHandoverV2{
									{
										Interval:     1,
										IntervalType: client.ScheduleRotationHandoverV2IntervalTypeDaily,
									},
								},
								Layers: []client.ScheduleLayerV2{
									{
										Id:   lo.ToPtr("layer-1"),
										Name: lo.ToPtr("Layer One"),
									},
								},
							},
							{
								Id:              "rota-test",
								Name:            "Test Rota",
								HandoverStartAt: handoverStartAt,
								Handovers: []client.ScheduleRotationHandoverV2{
									{
										Interval:     1,
										IntervalType: client.ScheduleRotationHandoverV2IntervalTypeDaily,
									},
								},
								EffectiveFrom: &effectiveFrom,
								Layers: []client.ScheduleLayerV2{
									{
										Id:   lo.ToPtr("layer-1"),
										Name: lo.ToPtr("Layer Two"),
									},
								},
								WorkingInterval: &[]client.ScheduleRotationWorkingIntervalV2{
									{
										EndTime:   "17:00",
										StartTime: "09:00",
										Weekday:   "monday",
									},
								},
							},
						},
					},
				}),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_schedule.example", "rotations.0.versions.#", "2",
					),
					testValueExistsInSet("incident_schedule.example", "rotations.0.versions.*.layers.0.name", "Layer One"),
					testValueExistsInSet("incident_schedule.example", "rotations.0.versions.*.layers.0.name", "Layer Two"),
					testValueExistsInSet(
						"incident_schedule.example", "rotations.0.versions.*.working_intervals.0.weekday", "monday",
					),
				),
			},
		},
	})
}

// Test that a value exists in a set, use '*' to replace the element indexing,
// this is different from the provided helpers as the set can be anywhere in
// the attribute path, rather than needing to be the last element.
func testValueExistsInSet(resourceName string, attr string, value string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("Not found: %s", resourceName)
		}
		is := rs.Primary
		if is == nil {
			return fmt.Errorf("No primary instance: %s in %s", resourceName, s.RootModule().Path)
		}

		attrParts := strings.Split(attr, ".")

		for stateKey, stateValue := range is.Attributes {
			stateKeyParts := strings.Split(stateKey, ".")

			if len(stateKeyParts) != len(attrParts) {
				continue
			}

			for i := range attrParts {
				if attrParts[i] != stateKeyParts[i] && attrParts[i] != "*" {
					break
				}
				if i == len(attrParts)-1 && stateValue == value {
					return nil
				}
			}
		}
		return fmt.Errorf("%s not found in %s", value, attr)
	}
}

func accIncidentScheduleResourceHolidayConfig(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create with holiday config
			{
				Config: testAccIncidentScheduleResourceConfig(&client.ScheduleV2{
					Name:     StableSuffix("holiday-test"),
					Timezone: "Europe/London",
					HolidaysPublicConfig: &client.ScheduleHolidaysPublicConfigV2{
						CountryCodes: []string{"GB", "FR"},
					},
				}),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_schedule.example", "holidays_public_config.country_codes.#", "2",
					),
					resource.TestCheckResourceAttr(
						"incident_schedule.example", "holidays_public_config.country_codes.0", "GB",
					),
					resource.TestCheckResourceAttr(
						"incident_schedule.example", "holidays_public_config.country_codes.1", "FR",
					),
				),
			},
			// Update holiday config
			{
				Config: testAccIncidentScheduleResourceConfig(&client.ScheduleV2{
					Name:     StableSuffix("holiday-test"),
					Timezone: "Europe/London",
					HolidaysPublicConfig: &client.ScheduleHolidaysPublicConfigV2{
						CountryCodes: []string{"GB", "DE"},
					},
				}),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_schedule.example", "holidays_public_config.country_codes.1", "DE",
					),
				),
			},
			// Remove holiday config
			{
				Config: testAccIncidentScheduleResourceConfig(&client.ScheduleV2{
					Name:     StableSuffix("holiday-test"),
					Timezone: "Europe/London",
				}),
				Check: resource.TestCheckResourceAttr(
					"incident_schedule.example", "holidays_public_config.#", "0",
				),
			},
		},
	})
}

func accIncidentScheduleResourceInvalidTimestamp(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				// Use a handcrafted config with an invalid timestamp to test validation
				Config: `
resource "incident_schedule" "invalid_timestamp" {
  name     = "invalid-timestamp-test"
  timezone = "Europe/London"
  rotations = [
    {
      id   = "test-rotation"
      name = "Test Rotation"
      versions = [
        {
          handover_start_at = "2024-15-11T15:00:00Z" # Invalid month (15)
					handovers = [
						{
							interval = 1,
							interval_type = "daily"
						}
					]
          users = []
          layers = [
            {
              id   = "test-layer"
              name = "Test Layer"
            }
          ]
        }
      ]
    }
  ]
}`,
				ExpectError: regexp.MustCompile("Invalid Timestamp Format"),
			},
			{
				// Test with invalid effective_from
				Config: `
resource "incident_schedule" "invalid_timestamp" {
  name     = "invalid-timestamp-test"
  timezone = "Europe/London"
  rotations = [
    {
      id   = "test-rotation"
      name = "Test Rotation"
      versions = [
        {
          handover_start_at = "2024-05-11T15:00:00Z"
					handovers = [
						{
							interval = 1,
							interval_type = "daily"
						}
					]
          effective_from   = "2024-05-32T15:00:00Z" # Invalid day (32)
          users = []
          layers = [
            {
              id   = "test-layer"
              name = "Test Layer"
            }
          ]
        }
      ]
    }
  ]
}`,
				ExpectError: regexp.MustCompile("Invalid Timestamp Format"),
			},
		},
	})
}

func incidentScheduleDefault(name string) client.ScheduleV2 {
	var (
		effectiveFrom1, _   = time.Parse(time.RFC3339, "2024-04-26T16:00:00Z")
		handoverStartAt1, _ = time.Parse(time.RFC3339, "2024-04-26T16:00:00Z")
	)

	return client.ScheduleV2{
		Name:     StableSuffix(name),
		Timezone: "Europe/London",
		Config: &client.ScheduleConfigV2{
			Rotations: []client.ScheduleRotationV2{
				{
					Id:              "rota-primary",
					EffectiveFrom:   &effectiveFrom1,
					HandoverStartAt: handoverStartAt1,
					Handovers: []client.ScheduleRotationHandoverV2{
						{
							IntervalType: client.ScheduleRotationHandoverV2IntervalType("weekly"),
							Interval:     1,
						},
					},
					Layers: []client.ScheduleLayerV2{
						{
							Id:   lo.ToPtr("rota-primary-layer-one"),
							Name: lo.ToPtr("Primary Layer One"),
						},
					},
					Name:  "Rota",
					Users: []client.UserV2{},
				},
			},
		},
		HolidaysPublicConfig: &client.ScheduleHolidaysPublicConfigV2{
			CountryCodes: []string{"GB", "FR"},
		},
	}
}

func quote(s string) string {
	return `"` + s + `"`
}

func generateScheduleTerraform(name string, schedule *client.ScheduleV2) string {
	var result string

	result += "resource \"incident_schedule\" \"example\" {\n"
	result += "  name     = " + quote(name) + "\n"
	result += "  timezone = " + quote(schedule.Timezone) + "\n"
	result += "  team_ids = " + generateTeamIDsArray(schedule.TeamIds) + "\n"
	result += "  " + generateRotationsArray(schedule.Config.Rotations)

	if schedule.HolidaysPublicConfig != nil {
		result += "  holidays_public_config = {\n"
		result += "    country_codes = " + generateCountryCodesArray(schedule.HolidaysPublicConfig.CountryCodes) + "\n"
		result += "  }\n"
	}

	result += "}\n"
	return result
}

func generateRotationsArray(rotations []client.ScheduleRotationV2) string {
	var result string

	rotationsByID := lo.GroupBy(rotations, func(rotation client.ScheduleRotationV2) string {
		return rotation.Id
	})

	result += "rotations = [\n"
	for _, rotation := range rotationsByID {

		if len(rotation) == 0 {
			continue
		}

		result += "  {\n"
		result += "    id              = " + quote(rotation[0].Id) + "\n"
		result += "    name            = " + quote(rotation[0].Name) + "\n"
		result += "    versions        = " + generateVersionsArray(rotation) + "\n"
		result += "  },\n"
	}
	result += "]\n"
	return result
}

func generateVersionsArray(versions []client.ScheduleRotationV2) string {
	var result string
	result += "[\n"
	for _, version := range versions {
		result += "  {\n"
		if version.EffectiveFrom != nil {
			result += "    effective_from   = " + quote(version.EffectiveFrom.Format(time.RFC3339)) + "\n"
		}
		result += "    handover_start_at = " + quote(version.HandoverStartAt.Format(time.RFC3339)) + "\n"
		result += "    handovers       = " + generateHandoversArray(version.Handovers) + "\n"
		result += "    layers          = " + generateLayersArray(version.Layers) + "\n"
		result += "    users           = " + generateUsersArray(version.Users) + "\n"
		if version.WorkingInterval != nil {
			result += "    working_intervals = " + generateWorkingIntervalsArray(version.WorkingInterval) + "\n"
		}
		result += "  },\n"
	}
	result += "]\n"
	return result
}

func generateHandoversArray(handovers []client.ScheduleRotationHandoverV2) string {
	var result string
	result += "[\n"
	for _, handover := range handovers {
		result += "  {\n"
		result += "    interval_type = " + quote(string(handover.IntervalType)) + "\n"
		result += "    interval      = " + quote(strconv.FormatInt(handover.Interval, 10)) + "\n"
		result += "  },\n"
	}
	result += "]\n"
	return result
}

func generateLayersArray(layers []client.ScheduleLayerV2) string {
	var result string
	result += "[\n"
	for _, layer := range layers {
		result += "  {\n"
		result += "    id   = " + quote(*layer.Id) + "\n"
		result += "    name = " + quote(*layer.Name) + "\n"
		result += "  },\n"
	}
	result += "]\n"
	return result
}

func generateUsersArray(users []client.UserV2) string {
	var result string
	if users == nil {
		return "[]"
	}
	result += "[\n"
	for _, user := range users {
		result += "  {\n"
		result += "    id   = " + quote(user.Id) + "\n"
		result += "  },\n"
	}
	result += "]\n"
	return result
}

func generateCountryCodesArray(codes []string) string {
	var result string
	if codes == nil {
		return "[]"
	}
	result += "["
	for idx, code := range codes {
		if idx > 0 {
			result += ", "
		}
		result += quote(code)
	}
	result += "]\n"
	return result
}

func generateTeamIDsArray(teamIDs []string) string {
	var result string
	if teamIDs == nil {
		return "[]"
	}
	result += "[\n"
	for idx, teamID := range teamIDs {
		if idx > 0 {
			result += ", "
		}
		result += quote(teamID)
	}
	result += "]\n"
	return result
}

func generateWorkingIntervalsArray(workingIntervals *[]client.ScheduleRotationWorkingIntervalV2) string {
	var result string
	result += "[\n"
	for _, workingInterval := range *workingIntervals {
		result += "  {\n"
		result += "    start_time = " + quote(workingInterval.StartTime) + "\n"
		result += "    end_time   = " + quote(workingInterval.EndTime) + "\n"
		result += "    weekday    = " + quote(string(workingInterval.Weekday)) + "\n"
		result += "  },\n"
	}
	result += "]\n"
	return result
}

func testAccIncidentScheduleResourceConfig(override *client.ScheduleV2) string {
	model := incidentScheduleDefault("ONC-Resource")

	// Merge any non-zero fields in override into the model.
	if override != nil {
		for idx := 0; idx < reflect.TypeOf(*override).NumField(); idx++ {
			field := reflect.ValueOf(*override).Field(idx)
			if !field.IsZero() {
				reflect.ValueOf(&model).Elem().Field(idx).Set(field)
			}
		}
	}

	// Use the model's name, not a literal: overrides set Name expecting it to reach the
	// config, and it was previously merged in and then ignored.
	terraformText := generateScheduleTerraform(model.Name, &model)

	return terraformText
}

// A config can write a timestamp as any valid RFC3339 string, and the dashboard's
// Terraform export writes them with millisecond precision, but the API returns
// every timestamp as UTC at second precision. Rotations and their versions are
// both sets, which Terraform correlates by raw value, so storing what the API
// returned rather than the literal the config asked for failed the post-apply
// consistency check on a schedule nothing had changed - and then planned that
// same change on every run.
//
// Both steps fail without that fix: the first on create, the second on the update
// that adds a version, which is how people hit it in practice. The test needs no
// explicit assertion of the round trip beyond the literals below, as the harness
// plans after each apply and fails on a plan that isn't empty.
func accIncidentScheduleResourceNonCanonicalTimestamps(t *testing.T) {
	var (
		name = StableSuffix("non-canonical-timestamps")
		// The same moment, written the two ways that broke: milliseconds, and an
		// offset that isn't UTC.
		firstHandoverStartAt  = "2024-05-01T12:00:00.000Z"
		secondEffectiveFrom   = "2027-03-01T09:00:00+02:00"
		secondHandoverStartAt = "2027-03-01T09:00:00+02:00"
	)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccIncidentScheduleTimestampConfig(name, firstHandoverStartAt, "", ""),
				Check: testValueExistsInSet(
					"incident_schedule.timestamps",
					"rotations.*.versions.*.handover_start_at", firstHandoverStartAt,
				),
			},
			// Schedule a change by adding a version, the way the reports we've had
			// arrived at this.
			{
				Config: testAccIncidentScheduleTimestampConfig(
					name, firstHandoverStartAt, secondEffectiveFrom, secondHandoverStartAt),
				Check: resource.ComposeAggregateTestCheckFunc(
					testValueExistsInSet(
						"incident_schedule.timestamps",
						"rotations.*.versions.*.handover_start_at", firstHandoverStartAt,
					),
					testValueExistsInSet(
						"incident_schedule.timestamps",
						"rotations.*.versions.*.effective_from", secondEffectiveFrom,
					),
				),
			},
		},
	})
}

// Versions of a rotation share an id in the API and we group them back together
// by id when reading a schedule, so spelling them as separate rotations can't
// round trip. That has to be an error while planning, because it applies quite
// happily and only shows up afterwards.
func accIncidentScheduleResourceDuplicateRotationID(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "incident_schedule" "duplicate_rotation_id" {
  name     = %q
  timezone = "Europe/London"

  rotations = [
    %s,
    %s,
  ]
}`,
					StableSuffix("duplicate-rotation-id"),
					testAccIncidentScheduleRotationBlock("weekdays", "2024-05-01T12:00:00Z", ""),
					testAccIncidentScheduleRotationBlock("weekdays", "2024-05-08T12:00:00Z", "2027-03-01T09:00:00Z"),
				),
				ExpectError: regexp.MustCompile("Duplicate rotation ID"),
			},
		},
	})
}

// testAccIncidentScheduleTimestampConfig builds a schedule with one rotation,
// holding a second version only when given one.
func testAccIncidentScheduleTimestampConfig(name, handoverStartAt, effectiveFrom, secondHandoverStartAt string) string {
	versions := testAccIncidentScheduleVersionBlock(handoverStartAt, "")
	if effectiveFrom != "" {
		versions += ",\n        " + testAccIncidentScheduleVersionBlock(secondHandoverStartAt, effectiveFrom)
	}

	return fmt.Sprintf(`
resource "incident_schedule" "timestamps" {
  name     = %q
  timezone = "Europe/London"

  rotations = [
    {
      id   = "weekdays"
      name = "Weekdays"
      versions = [
        %s,
      ]
    },
  ]
}`, name, versions)
}

func testAccIncidentScheduleRotationBlock(id, handoverStartAt, effectiveFrom string) string {
	return fmt.Sprintf(`{
      id   = %q
      name = "Weekdays"
      versions = [
        %s,
      ]
    }`, id, testAccIncidentScheduleVersionBlock(handoverStartAt, effectiveFrom))
}

// testAccIncidentScheduleVersionBlock writes a rotation version, using
// INCIDENT_TEST_USER_ID for its line-up where the environment names a user: the
// reports we've had all came from rotations that have people in them, and an
// empty rotation exercises less of the read path.
func testAccIncidentScheduleVersionBlock(handoverStartAt, effectiveFrom string) string {
	users := "[]"
	if userID := os.Getenv("INCIDENT_TEST_USER_ID"); userID != "" {
		users = fmt.Sprintf("[%q]", userID)
	}

	version := fmt.Sprintf(`{
          handover_start_at = %q
          users             = %s
          layers            = [{ id = "weekdays-one", name = "Layer 1" }]
          handovers         = [{ interval = 1, interval_type = "weekly" }]`, handoverStartAt, users)

	if effectiveFrom != "" {
		version += fmt.Sprintf(`
          effective_from    = %q`, effectiveFrom)
	}

	return version + `
        }`
}
