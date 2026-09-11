package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

// TestParseScheduleReplicaImportID covers the composite import ID a replica is
// identified by: both the schedule and the replica ID are required, since
// replicas are nested under a schedule in the API.
func TestParseScheduleReplicaImportID(t *testing.T) {
	cases := []struct {
		name           string
		importID       string
		wantScheduleID string
		wantReplicaID  string
		wantOK         bool
	}{
		{
			name:           "schedule and replica ID",
			importID:       "01ABC123DEF456GHI789JKL:01MNO456PQR789STU012VWX",
			wantScheduleID: "01ABC123DEF456GHI789JKL",
			wantReplicaID:  "01MNO456PQR789STU012VWX",
			wantOK:         true,
		},
		{
			name:           "surrounding whitespace",
			importID:       " 01ABC123DEF456GHI789JKL : 01MNO456PQR789STU012VWX ",
			wantScheduleID: "01ABC123DEF456GHI789JKL",
			wantReplicaID:  "01MNO456PQR789STU012VWX",
			wantOK:         true,
		},
		{
			name:     "replica ID only",
			importID: "01MNO456PQR789STU012VWX",
		},
		{
			name:     "missing schedule ID",
			importID: ":01MNO456PQR789STU012VWX",
		},
		{
			name:     "missing replica ID",
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
			scheduleID, replicaID, ok := parseScheduleReplicaImportID(tc.importID)
			if ok != tc.wantOK {
				t.Fatalf("parseScheduleReplicaImportID(%q) ok = %v, want %v", tc.importID, ok, tc.wantOK)
			}
			if !tc.wantOK {
				return
			}
			if scheduleID != tc.wantScheduleID {
				t.Errorf("schedule ID = %q, want %q", scheduleID, tc.wantScheduleID)
			}
			if replicaID != tc.wantReplicaID {
				t.Errorf("replica ID = %q, want %q", replicaID, tc.wantReplicaID)
			}
		})
	}
}

func TestIncidentScheduleReplicaSchemas(t *testing.T) {
	var resourceResp resource.SchemaResponse
	NewIncidentScheduleReplicaResource().Schema(context.Background(), resource.SchemaRequest{}, &resourceResp)
	if resourceResp.Diagnostics.HasError() {
		t.Fatalf("resource schema: %+v", resourceResp.Diagnostics)
	}

	var dataSourceResp datasource.SchemaResponse
	NewIncidentScheduleReplicaDataSource().Schema(context.Background(), datasource.SchemaRequest{}, &dataSourceResp)
	if dataSourceResp.Diagnostics.HasError() {
		t.Fatalf("data source schema: %+v", dataSourceResp.Diagnostics)
	}
}
