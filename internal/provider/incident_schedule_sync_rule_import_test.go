package provider

import "testing"

// TestParseScheduleSyncRuleImportID covers the composite import ID a sync rule
// is identified by: both the schedule and the rule ID are required, since rules
// are nested under a schedule in the API.
func TestParseScheduleSyncRuleImportID(t *testing.T) {
	cases := []struct {
		name           string
		importID       string
		wantScheduleID string
		wantRuleID     string
		wantOK         bool
	}{
		{
			name:           "schedule and rule ID",
			importID:       "01ABC123DEF456GHI789JKL:01MNO456PQR789STU012VWX",
			wantScheduleID: "01ABC123DEF456GHI789JKL",
			wantRuleID:     "01MNO456PQR789STU012VWX",
			wantOK:         true,
		},
		{
			name:           "surrounding whitespace",
			importID:       " 01ABC123DEF456GHI789JKL : 01MNO456PQR789STU012VWX ",
			wantScheduleID: "01ABC123DEF456GHI789JKL",
			wantRuleID:     "01MNO456PQR789STU012VWX",
			wantOK:         true,
		},
		{
			name:     "rule ID only",
			importID: "01MNO456PQR789STU012VWX",
		},
		{
			name:     "missing schedule ID",
			importID: ":01MNO456PQR789STU012VWX",
		},
		{
			name:     "missing rule ID",
			importID: "01ABC123DEF456GHI789JKL:",
		},
		{
			name:     "too many parts",
			importID: "01ABC123DEF456GHI789JKL:01MNO456PQR789STU012VWX:extra",
		},
		{
			name:     "empty",
			importID: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			scheduleID, ruleID, ok := parseScheduleSyncRuleImportID(tc.importID)
			if ok != tc.wantOK {
				t.Fatalf("parseScheduleSyncRuleImportID(%q) ok = %v, want %v", tc.importID, ok, tc.wantOK)
			}
			if !tc.wantOK {
				return
			}
			if scheduleID != tc.wantScheduleID {
				t.Errorf("schedule ID = %q, want %q", scheduleID, tc.wantScheduleID)
			}
			if ruleID != tc.wantRuleID {
				t.Errorf("rule ID = %q, want %q", ruleID, tc.wantRuleID)
			}
		})
	}
}
