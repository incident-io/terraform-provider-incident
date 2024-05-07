package provider

import (
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/incident-io/terraform-provider-incident/internal/client"
	"github.com/samber/lo"
)

func TestAccIncidentScheduleResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and read
			{
				Config: testAccIncidentScheduleResourceConfig(&client.ScheduleV2{
					Name:     "example",
					Timezone: "Europe/London",
					Config: &client.ScheduleConfigV2{
						Rotations: []client.ScheduleRotationV2{
							{
								Id:              "rota-primary",
								HandoverStartAt: time.Date(2024, 4, 26, 16, 0, 0, 0, time.UTC),
								Name:            "Rota",
								Handovers: []client.ScheduleRotationHandoverV2{
									{
										IntervalType: lo.ToPtr(client.ScheduleRotationHandoverV2IntervalType("weekly")),
										Interval:     lo.ToPtr(int64(1)),
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
								EffectiveFrom:   lo.ToPtr(time.Now().Add(time.Hour * 24)),
								Name:            "Rota",
								Handovers: []client.ScheduleRotationHandoverV2{
									{
										IntervalType: lo.ToPtr(client.ScheduleRotationHandoverV2IntervalType("weekly")),
										Interval:     lo.ToPtr(int64(1)),
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
						"incident_schedule.example", "name", "example"),
					resource.TestCheckResourceAttr(
						"incident_schedule.example", "timezone", "Europe/London",
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

func incidentScheduleDefault() client.ScheduleV2 {
	var (
		effectiveFrom1, _   = time.Parse(time.RFC3339, "2024-04-26T16:00:00Z")
		handoverStartAt1, _ = time.Parse(time.RFC3339, "2024-04-26T16:00:00Z")
	)

	return client.ScheduleV2{
		Name:     "ONC",
		Timezone: "Europe/London",
		Config: &client.ScheduleConfigV2{
			Rotations: []client.ScheduleRotationV2{
				{
					Id:              "rota-primary",
					EffectiveFrom:   &effectiveFrom1,
					HandoverStartAt: handoverStartAt1,
					Handovers: []client.ScheduleRotationHandoverV2{
						{
							IntervalType: lo.ToPtr(client.ScheduleRotationHandoverV2IntervalType("weekly")),
							Interval:     lo.ToPtr(int64(1)),
						},
					},
					Layers: []client.ScheduleLayerV2{
						{
							Id:   lo.ToPtr("rota-primary-layer-one"),
							Name: lo.ToPtr("Primary Layer One"),
						},
					},
					Name:            "Rota",
					Users:           new([]client.UserV1),
					WorkingInterval: nil,
				},
			},
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
	result += "  " + generateRotationsArray(schedule.Config.Rotations)
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
		result += "    handover_start_at = " + quote(rotation[0].HandoverStartAt.Format(time.RFC3339)) + "\n"
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
		result += "    handovers       = " + generateHandoversArray(version.Handovers) + "\n"
		result += "    layers          = " + generateLayersArray(version.Layers) + "\n"
		result += "    users           = " + generateUsersArray(version.Users) + "\n"
		if version.WorkingInterval != nil {
			result += "    working_interval = " + generateWorkingIntervalsArray(version.WorkingInterval) + "\n"
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
		result += "    interval_type = " + quote(string(*handover.IntervalType)) + "\n"
		result += "    interval      = " + quote(strconv.FormatInt(*handover.Interval, 10)) + "\n"
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

func generateUsersArray(users *[]client.UserV1) string {
	var result string
	if users == nil {
		return "[]"
	}
	result += "[\n"
	for _, user := range *users {
		result += "  {\n"
		result += "    id   = " + quote(user.Id) + "\n"
		result += "  },\n"
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
	model := incidentScheduleDefault()

	// Merge any non-zero fields in override into the model.
	if override != nil {
		for idx := 0; idx < reflect.TypeOf(*override).NumField(); idx++ {
			field := reflect.ValueOf(*override).Field(idx)
			if !field.IsZero() {
				reflect.ValueOf(&model).Elem().Field(idx).Set(field)
			}
		}
	}

	terraformText := generateScheduleTerraform("example", &model)

	return terraformText
}
