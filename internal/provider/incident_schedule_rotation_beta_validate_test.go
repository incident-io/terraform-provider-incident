package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// workingIntervalObjectType mirrors the nested object in the schema.
var workingIntervalObjectType = tftypes.Object{
	AttributeTypes: map[string]tftypes.Type{
		"weekday":    tftypes.String,
		"start_time": tftypes.String,
		"end_time":   tftypes.String,
	},
}

var workingIntervalsListType = tftypes.List{ElementType: workingIntervalObjectType}

var handoverObjectType = tftypes.Object{
	AttributeTypes: map[string]tftypes.Type{
		"interval":      tftypes.Number,
		"interval_type": tftypes.String,
	},
}

func handoversValue(handovers ...tftypes.Value) tftypes.Value {
	return tftypes.NewValue(tftypes.List{ElementType: handoverObjectType}, handovers)
}

func handoverValue(interval int64, intervalType string) tftypes.Value {
	return tftypes.NewValue(handoverObjectType, map[string]tftypes.Value{
		"interval":      tftypes.NewValue(tftypes.Number, interval),
		"interval_type": tftypes.NewValue(tftypes.String, intervalType),
	})
}

func workingIntervalsValue(intervals ...tftypes.Value) tftypes.Value {
	return tftypes.NewValue(workingIntervalsListType, intervals)
}

func workingIntervalValue(weekday, startTime, endTime string) tftypes.Value {
	return tftypes.NewValue(workingIntervalObjectType, map[string]tftypes.Value{
		"weekday":    tftypes.NewValue(tftypes.String, weekday),
		"start_time": tftypes.NewValue(tftypes.String, startTime),
		"end_time":   tftypes.NewValue(tftypes.String, endTime),
	})
}

// validateScheduleRotation builds a tfsdk.Config against the real resource schema, so
// this exercises ValidateConfig exactly as Terraform calls it on every plan.
//
// The base config is a valid rotation, with overrides replacing attributes by name. Every
// check runs on every plan, so a case for one has to leave the rest of the config valid.
func validateScheduleRotation(t *testing.T, overrides map[string]tftypes.Value) resource.ValidateConfigResponse {
	t.Helper()

	var schemaResp resource.SchemaResponse
	NewIncidentScheduleRotationBetaResource().Schema(context.Background(), resource.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("schema build failed: %+v", schemaResp.Diagnostics)
	}

	objType, ok := schemaResp.Schema.Type().TerraformType(context.Background()).(tftypes.Object)
	if !ok {
		t.Fatalf("schema type is not an object")
	}

	attributes := map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "01ROTA"),
		"schedule_id": tftypes.NewValue(tftypes.String, "01SCHED"),
		"name":        tftypes.NewValue(tftypes.String, "Primary"),
		"users": tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, []tftypes.Value{
			tftypes.NewValue(tftypes.String, "01USER"),
		}),
		"handovers":                handoversValue(handoverValue(1, "weekly")),
		"first_interval_starts_at": tftypes.NewValue(tftypes.String, "2024-01-08T09:00:00Z"),
		"concurrent_shifts":        tftypes.NewValue(tftypes.Number, 1),
		"working_intervals":        tftypes.NewValue(workingIntervalsListType, nil),
		"rank":                     tftypes.NewValue(tftypes.Number, nil),
		"scheduling_mode":          tftypes.NewValue(tftypes.String, nil),
		"rollout":                  tftypes.NewValue(tftypes.String, nil),
		"effective_from":           tftypes.NewValue(tftypes.String, nil),
	}
	for name, value := range overrides {
		if _, ok := attributes[name]; !ok {
			t.Fatalf("override %q isn't an attribute on the rotation schema", name)
		}
		attributes[name] = value
	}

	config := tfsdk.Config{
		Schema: schemaResp.Schema,
		Raw:    tftypes.NewValue(objType, attributes),
	}

	r, ok := NewIncidentScheduleRotationBetaResource().(*IncidentScheduleRotationBetaResource)
	if !ok {
		t.Fatalf("NewIncidentScheduleRotationBetaResource did not return a *IncidentScheduleRotationBetaResource")
	}
	var resp resource.ValidateConfigResponse
	r.ValidateConfig(context.Background(), resource.ValidateConfigRequest{Config: config}, &resp)
	return resp
}

// expectValid and expectInvalid keep each case to the config it's about.
func expectValid(t *testing.T, overrides map[string]tftypes.Value) {
	t.Helper()
	if resp := validateScheduleRotation(t, overrides); resp.Diagnostics.HasError() {
		t.Errorf("unexpected diagnostics: %+v", resp.Diagnostics)
	}
}

func expectInvalid(t *testing.T, overrides map[string]tftypes.Value) {
	t.Helper()
	if resp := validateScheduleRotation(t, overrides); !resp.Diagnostics.HasError() {
		t.Error("expected an error, got none")
	}
}

func TestScheduleRotationValidateConfig(t *testing.T) {
	t.Run("the base config is valid", func(t *testing.T) {
		expectValid(t, nil)
	})

	// Judging each attribute on the response's diagnostics rather than its own would
	// report the first and hide the second, sending someone round the loop twice for one
	// plan's worth of mistakes.
	t.Run("two bad attributes both report", func(t *testing.T) {
		resp := validateScheduleRotation(t, map[string]tftypes.Value{
			"working_intervals": workingIntervalsValue(),
			"rollout":           tftypes.NewValue(tftypes.String, "tomorrow"),
		})
		if len(resp.Diagnostics.Errors()) != 2 {
			t.Errorf("expected both errors, got %+v", resp.Diagnostics)
		}
	})

	// Two of the checks added later, to be sure the same holds across all of them and not
	// just the original pair.
	t.Run("a bad name and a bad rank both report", func(t *testing.T) {
		resp := validateScheduleRotation(t, map[string]tftypes.Value{
			"name": tftypes.NewValue(tftypes.String, ""),
			"rank": tftypes.NewValue(tftypes.Number, 0),
		})
		if len(resp.Diagnostics.Errors()) != 2 {
			t.Errorf("expected both errors, got %+v", resp.Diagnostics)
		}
	})
}

func TestScheduleRotationValidateRollout(t *testing.T) {
	for _, rollout := range scheduleRotationRollouts {
		t.Run(rollout+" is accepted", func(t *testing.T) {
			expectValid(t, map[string]tftypes.Value{
				"rollout": tftypes.NewValue(tftypes.String, rollout),
			})
		})
	}

	// Caught here rather than by the API, so a typo fails the plan instead of an apply
	// that has already changed other resources.
	t.Run("an unknown rollout is rejected", func(t *testing.T) {
		expectInvalid(t, map[string]tftypes.Value{
			"rollout": tftypes.NewValue(tftypes.String, "tomorrow"),
		})
	})

	t.Run("an unknown-valued rollout is left alone", func(t *testing.T) {
		expectValid(t, map[string]tftypes.Value{
			"rollout": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		})
	})
}

func TestScheduleRotationValidateSchedulingMode(t *testing.T) {
	// Read from the vendored schema rather than listed here, so this can't pass against
	// a set of values the API has since moved on from.
	for _, mode := range scheduleRotationSchedulingModes {
		t.Run(mode+" is accepted", func(t *testing.T) {
			expectValid(t, map[string]tftypes.Value{
				"scheduling_mode": tftypes.NewValue(tftypes.String, mode),
			})
		})
	}

	t.Run("an unknown scheduling_mode is rejected", func(t *testing.T) {
		expectInvalid(t, map[string]tftypes.Value{
			"scheduling_mode": tftypes.NewValue(tftypes.String, "round_robin"),
		})
	})

	// Unknown rather than absent, which is what a Computed attribute looks like before
	// apply fills it in — there's nothing to judge yet.
	t.Run("an unknown-valued scheduling_mode is left alone", func(t *testing.T) {
		expectValid(t, map[string]tftypes.Value{
			"scheduling_mode": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		})
	})
}

func TestScheduleRotationValidateName(t *testing.T) {
	// Required only makes Terraform insist the attribute is set, so an empty string gets
	// as far as the domain before anything objects.
	t.Run("an empty name is rejected", func(t *testing.T) {
		expectInvalid(t, map[string]tftypes.Value{
			"name": tftypes.NewValue(tftypes.String, ""),
		})
	})

	// The domain compares against "", so a name of spaces is one it accepts.
	t.Run("a name of spaces is left alone", func(t *testing.T) {
		expectValid(t, map[string]tftypes.Value{
			"name": tftypes.NewValue(tftypes.String, " "),
		})
	})

	t.Run("an unknown name is left alone", func(t *testing.T) {
		expectValid(t, map[string]tftypes.Value{
			"name": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		})
	})
}

func TestScheduleRotationValidateUsers(t *testing.T) {
	usersListType := tftypes.List{ElementType: tftypes.String}

	// The API takes an empty list and then schedules nobody, so this is the only place
	// the mistake shows up.
	t.Run("an empty users is rejected", func(t *testing.T) {
		expectInvalid(t, map[string]tftypes.Value{
			"users": tftypes.NewValue(usersListType, []tftypes.Value{}),
		})
	})

	t.Run("an unknown users is left alone", func(t *testing.T) {
		expectValid(t, map[string]tftypes.Value{
			"users": tftypes.NewValue(usersListType, tftypes.UnknownValue),
		})
	})
}

func TestScheduleRotationValidateHandovers(t *testing.T) {
	t.Run("an empty handovers is rejected", func(t *testing.T) {
		expectInvalid(t, map[string]tftypes.Value{"handovers": handoversValue()})
	})

	t.Run("an unrecognised interval_type is rejected", func(t *testing.T) {
		expectInvalid(t, map[string]tftypes.Value{
			"handovers": handoversValue(handoverValue(1, "monthly")),
		})
	})

	// The domain caps the repeat per interval type, and rejects it with a 422 partway
	// through an apply.
	t.Run("the repeat is bounded by interval type", func(t *testing.T) {
		valid := []tftypes.Value{
			handoverValue(1, "weekly"),
			handoverValue(4, "weekly"),
			handoverValue(14, "daily"),
			handoverValue(23, "hourly"),
		}
		for _, handover := range valid {
			expectValid(t, map[string]tftypes.Value{"handovers": handoversValue(handover)})
		}

		invalid := []tftypes.Value{
			handoverValue(0, "weekly"),
			handoverValue(-1, "weekly"),
			handoverValue(5, "weekly"),
			handoverValue(15, "daily"),
			handoverValue(24, "hourly"),
		}
		for _, handover := range invalid {
			expectInvalid(t, map[string]tftypes.Value{"handovers": handoversValue(handover)})
		}
	})

	t.Run("alternating handovers are each checked", func(t *testing.T) {
		expectValid(t, map[string]tftypes.Value{
			"handovers": handoversValue(handoverValue(2, "daily"), handoverValue(5, "daily")),
		})
		expectInvalid(t, map[string]tftypes.Value{
			"handovers": handoversValue(handoverValue(2, "daily"), handoverValue(15, "daily")),
		})
	})

	// A handover built from another resource's output isn't knowable at plan time.
	t.Run("an unknown handover is left alone", func(t *testing.T) {
		expectValid(t, map[string]tftypes.Value{
			"handovers": handoversValue(tftypes.NewValue(handoverObjectType, tftypes.UnknownValue)),
		})
		expectValid(t, map[string]tftypes.Value{
			"handovers": handoversValue(tftypes.NewValue(handoverObjectType, map[string]tftypes.Value{
				"interval":      tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
				"interval_type": tftypes.NewValue(tftypes.String, "weekly"),
			})),
		})
	})
}

func TestScheduleRotationValidateWorkingIntervals(t *testing.T) {
	t.Run("no working_intervals is fine", func(t *testing.T) {
		expectValid(t, map[string]tftypes.Value{
			"working_intervals": tftypes.NewValue(workingIntervalsListType, nil),
		})
	})

	t.Run("a populated working_intervals is fine", func(t *testing.T) {
		expectValid(t, map[string]tftypes.Value{
			"working_intervals": workingIntervalsValue(workingIntervalValue("monday", "09:00", "17:00")),
		})
	})

	// Absent means "on call around the clock", so an empty list is someone reaching for
	// that and getting a rotation nobody is ever on call for.
	t.Run("an empty working_intervals is rejected", func(t *testing.T) {
		expectInvalid(t, map[string]tftypes.Value{"working_intervals": workingIntervalsValue()})
	})

	// An unknown list comes from another resource's output and can't be judged at plan
	// time. Unknown is not null, so a missing guard would reject a legitimate config.
	t.Run("an unknown working_intervals is left alone", func(t *testing.T) {
		expectValid(t, map[string]tftypes.Value{
			"working_intervals": tftypes.NewValue(workingIntervalsListType, tftypes.UnknownValue),
		})
	})

	t.Run("an unrecognised weekday is rejected", func(t *testing.T) {
		expectInvalid(t, map[string]tftypes.Value{
			"working_intervals": workingIntervalsValue(workingIntervalValue("Monday", "09:00", "17:00")),
		})
	})

	t.Run("a malformed time of day is rejected", func(t *testing.T) {
		malformed := []tftypes.Value{
			workingIntervalValue("monday", "9:00", "17:00"),
			workingIntervalValue("monday", "09:00", "17:00:00"),
			workingIntervalValue("monday", "09:00", "24:00"),
			workingIntervalValue("monday", "09:60", "17:00"),
			workingIntervalValue("monday", "0900", "17:00"),
			workingIntervalValue("monday", "", "17:00"),
		}
		for _, interval := range malformed {
			expectInvalid(t, map[string]tftypes.Value{
				"working_intervals": workingIntervalsValue(interval),
			})
		}
	})

	t.Run("two windows on one weekday must not overlap", func(t *testing.T) {
		expectInvalid(t, map[string]tftypes.Value{
			"working_intervals": workingIntervalsValue(
				workingIntervalValue("monday", "09:00", "17:00"),
				workingIntervalValue("monday", "16:00", "20:00"),
			),
		})
	})

	t.Run("windows that only touch are fine", func(t *testing.T) {
		expectValid(t, map[string]tftypes.Value{
			"working_intervals": workingIntervalsValue(
				workingIntervalValue("monday", "09:00", "12:00"),
				workingIntervalValue("monday", "12:00", "17:00"),
			),
		})
	})

	t.Run("the same window on different weekdays is fine", func(t *testing.T) {
		expectValid(t, map[string]tftypes.Value{
			"working_intervals": workingIntervalsValue(
				workingIntervalValue("monday", "09:00", "17:00"),
				workingIntervalValue("tuesday", "09:00", "17:00"),
			),
		})
	})

	// A window that runs past midnight belongs partly to the next day, and the API's own
	// overlap check doesn't spot a clash involving one — so we don't either, rather than
	// reject a config it accepts.
	t.Run("a window running past midnight is not judged for overlap", func(t *testing.T) {
		expectValid(t, map[string]tftypes.Value{
			"working_intervals": workingIntervalsValue(
				workingIntervalValue("monday", "17:00", "09:00"),
				workingIntervalValue("monday", "18:00", "20:00"),
			),
		})
	})

	t.Run("an unknown window takes no part in the overlap check", func(t *testing.T) {
		expectValid(t, map[string]tftypes.Value{
			"working_intervals": workingIntervalsValue(
				workingIntervalValue("monday", "09:00", "17:00"),
				tftypes.NewValue(workingIntervalObjectType, map[string]tftypes.Value{
					"weekday":    tftypes.NewValue(tftypes.String, "monday"),
					"start_time": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
					"end_time":   tftypes.NewValue(tftypes.String, "20:00"),
				}),
			),
		})
	})
}

func TestScheduleRotationValidateConcurrentShiftsAndRank(t *testing.T) {
	t.Run("concurrent_shifts is bounded", func(t *testing.T) {
		for _, shifts := range []int64{1, 20} {
			expectValid(t, map[string]tftypes.Value{
				"concurrent_shifts": tftypes.NewValue(tftypes.Number, shifts),
			})
		}
		for _, shifts := range []int64{0, -1, 21} {
			expectInvalid(t, map[string]tftypes.Value{
				"concurrent_shifts": tftypes.NewValue(tftypes.Number, shifts),
			})
		}
	})

	// Rank counts from one, and zero is how the config records a rotation that has never
	// been ordered — so it's an omitted attribute, not a position you can ask for.
	t.Run("rank counts from one", func(t *testing.T) {
		expectValid(t, map[string]tftypes.Value{
			"rank": tftypes.NewValue(tftypes.Number, 1),
		})
		for _, rank := range []int64{0, -1} {
			expectInvalid(t, map[string]tftypes.Value{
				"rank": tftypes.NewValue(tftypes.Number, rank),
			})
		}
	})
}

// validateScheduleRotationDataSource runs the data source's ValidateConfig against
// the real schema, with id and name set to the given raw values.
func validateScheduleRotationDataSource(t *testing.T, id, name tftypes.Value) datasource.ValidateConfigResponse {
	t.Helper()

	var schemaResp datasource.SchemaResponse
	NewIncidentScheduleRotationBetaDataSource().Schema(context.Background(), datasource.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("schema build failed: %+v", schemaResp.Diagnostics)
	}

	objType, ok := schemaResp.Schema.Type().TerraformType(context.Background()).(tftypes.Object)
	if !ok {
		t.Fatalf("schema type is not an object")
	}
	config := tfsdk.Config{
		Schema: schemaResp.Schema,
		Raw: tftypes.NewValue(objType, map[string]tftypes.Value{
			"schedule_id":              tftypes.NewValue(tftypes.String, "01SCHED"),
			"id":                       id,
			"name":                     name,
			"users":                    tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, nil),
			"handovers":                tftypes.NewValue(tftypes.List{ElementType: handoverObjectType}, nil),
			"first_interval_starts_at": tftypes.NewValue(tftypes.String, nil),
			"concurrent_shifts":        tftypes.NewValue(tftypes.Number, nil),
			"working_intervals":        tftypes.NewValue(workingIntervalsListType, nil),
			"rank":                     tftypes.NewValue(tftypes.Number, nil),
			"scheduling_mode":          tftypes.NewValue(tftypes.String, nil),
			"effective_from":           tftypes.NewValue(tftypes.String, nil),
		}),
	}

	d, ok := NewIncidentScheduleRotationBetaDataSource().(*IncidentScheduleRotationBetaDataSource)
	if !ok {
		t.Fatalf("NewIncidentScheduleRotationBetaDataSource did not return a *IncidentScheduleRotationBetaDataSource")
	}
	var resp datasource.ValidateConfigResponse
	d.ValidateConfig(context.Background(), datasource.ValidateConfigRequest{Config: config}, &resp)
	return resp
}

func TestScheduleRotationDataSourceValidateConfig(t *testing.T) {
	null := tftypes.NewValue(tftypes.String, nil)
	unknown := tftypes.NewValue(tftypes.String, tftypes.UnknownValue)
	value := tftypes.NewValue(tftypes.String, "01ROTA")

	t.Run("id alone is fine", func(t *testing.T) {
		if resp := validateScheduleRotationDataSource(t, value, null); resp.Diagnostics.HasError() {
			t.Errorf("unexpected diagnostics: %+v", resp.Diagnostics)
		}
	})

	t.Run("name alone is fine", func(t *testing.T) {
		if resp := validateScheduleRotationDataSource(t, null, value); resp.Diagnostics.HasError() {
			t.Errorf("unexpected diagnostics: %+v", resp.Diagnostics)
		}
	})

	t.Run("both set is rejected", func(t *testing.T) {
		if resp := validateScheduleRotationDataSource(t, value, value); !resp.Diagnostics.HasError() {
			t.Error("expected an error when both id and name are set")
		}
	})

	t.Run("neither set is rejected", func(t *testing.T) {
		if resp := validateScheduleRotationDataSource(t, null, null); !resp.Diagnostics.HasError() {
			t.Error("expected an error when neither id nor name is set")
		}
	})

	// An id taken from a resource created in the same apply isn't known at plan time,
	// and unknown is not null — so judging it would reject a config that's fine.
	t.Run("an unknown lookup is left alone", func(t *testing.T) {
		if resp := validateScheduleRotationDataSource(t, unknown, null); resp.Diagnostics.HasError() {
			t.Errorf("unexpected diagnostics: %+v", resp.Diagnostics)
		}
	})
}
