package provider

import (
	"os"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// TestAccIncidentScheduleReplicaResource creates a schedule replica against a
// real external provider schedule.
//
// This needs an external on-call integration (PagerDuty, Opsgenie, or JSM) on
// the test account. Set TF_ACC_SCHEDULE_REPLICAS=1, plus:
//
//	TF_ACC_REPLICA_PROVIDER          e.g. pagerduty
//	TF_ACC_REPLICA_PROVIDER_ID       the external schedule ID
//	TF_ACC_REPLICA_FALLBACK_USER_ID  a user ID in that external provider
//	TF_ACC_REPLICA_LAYER_ID          a layer ID on the rotation this creates
//
// The layer ID has to be supplied because nothing in the configuration can produce
// one. A replica source names the rotation layer it mirrors, and layer IDs are
// assigned by the API: incident_schedule used to let you choose them, and
// incident_schedule_rotation does not expose the ones it is given. Until it does,
// running this means creating the rotation, reading a layer ID off the API, and
// setting it here.
func TestAccIncidentScheduleReplicaResource(t *testing.T) {
	if os.Getenv("TF_ACC_SCHEDULE_REPLICAS") == "" {
		t.Skip("TF_ACC_SCHEDULE_REPLICAS is not set: skipping test that requires an external on-call integration")
	}

	replicaProvider := os.Getenv("TF_ACC_REPLICA_PROVIDER")
	replicaProviderID := os.Getenv("TF_ACC_REPLICA_PROVIDER_ID")
	fallbackUserID := os.Getenv("TF_ACC_REPLICA_FALLBACK_USER_ID")
	layerID := os.Getenv("TF_ACC_REPLICA_LAYER_ID")
	if replicaProvider == "" || replicaProviderID == "" || fallbackUserID == "" || layerID == "" {
		t.Fatal("TF_ACC_REPLICA_PROVIDER, TF_ACC_REPLICA_PROVIDER_ID, TF_ACC_REPLICA_FALLBACK_USER_ID and TF_ACC_REPLICA_LAYER_ID must be set")
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccScheduleReplicaResourceConfig(replicaProvider, replicaProviderID, fallbackUserID, layerID, 14),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_schedule_replica.test", "replica_provider", replicaProvider),
					resource.TestCheckResourceAttr("incident_schedule_replica.test", "replica_provider_id", replicaProviderID),
					resource.TestCheckResourceAttr("incident_schedule_replica.test", "replica_fallback_user_id", fallbackUserID),
					resource.TestCheckResourceAttr("incident_schedule_replica.test", "mirror_window_days", "14"),
					resource.TestCheckResourceAttr("incident_schedule_replica.test", "sources.#", "1"),
					resource.TestCheckResourceAttrSet("incident_schedule_replica.test", "id"),
					resource.TestCheckResourceAttrSet("incident_schedule_replica.test", "schedule_id"),
					resource.TestCheckResourceAttrPair(
						"data.incident_schedule_replica.test", "id",
						"incident_schedule_replica.test", "id",
					),
					resource.TestCheckResourceAttrPair(
						"data.incident_schedule_replica.test", "replica_provider_id",
						"incident_schedule_replica.test", "replica_provider_id",
					),
				),
			},
			{
				ResourceName:      "incident_schedule_replica.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: importScheduleReplicaStateIDFunc("incident_schedule_replica.test"),
			},
			{
				Config: testAccScheduleReplicaResourceConfig(replicaProvider, replicaProviderID, fallbackUserID, layerID, 21),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_schedule_replica.test", "mirror_window_days", "21"),
				),
			},
		},
	})
}

func TestAccIncidentScheduleReplicaResource_InvalidImportID(t *testing.T) {
	if os.Getenv("TF_ACC_SCHEDULE_REPLICAS") == "" {
		t.Skip("TF_ACC_SCHEDULE_REPLICAS is not set: skipping test that requires an external on-call integration")
	}

	replicaProvider := os.Getenv("TF_ACC_REPLICA_PROVIDER")
	replicaProviderID := os.Getenv("TF_ACC_REPLICA_PROVIDER_ID")
	fallbackUserID := os.Getenv("TF_ACC_REPLICA_FALLBACK_USER_ID")
	layerID := os.Getenv("TF_ACC_REPLICA_LAYER_ID")
	if replicaProvider == "" || replicaProviderID == "" || fallbackUserID == "" || layerID == "" {
		t.Fatal("TF_ACC_REPLICA_PROVIDER, TF_ACC_REPLICA_PROVIDER_ID, TF_ACC_REPLICA_FALLBACK_USER_ID and TF_ACC_REPLICA_LAYER_ID must be set")
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccScheduleReplicaResourceConfig(replicaProvider, replicaProviderID, fallbackUserID, layerID, 14),
			},
			{
				ResourceName:  "incident_schedule_replica.test",
				ImportState:   true,
				ImportStateId: "invalid-id-without-colon",
				ExpectError:   regexp.MustCompile(`The import ID must be in the format: schedule_id:replica_id`),
			},
		},
	})
}

func importScheduleReplicaStateIDFunc(resourceName string) resource.ImportStateIdFunc {
	return func(s *terraform.State) (string, error) {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return "", nil
		}
		return rs.Primary.Attributes["schedule_id"] + ":" + rs.Primary.ID, nil
	}
}

func testAccScheduleReplicaResourceConfig(replicaProvider, replicaProviderID, fallbackUserID, layerID string, mirrorWindowDays int) string {
	return testRunTemplate("incident_schedule_replica", `
resource "incident_schedule" "test" {
  name     = {{ stableSuffix "Test Schedule for Replica" | quote }}
  timezone = "Europe/London"
}

resource "incident_schedule_rotation" "test" {
  schedule_id = incident_schedule.test.id
  name        = "Primary"

  users = ["NOBODY"]

  first_interval_starts_at = "2024-05-01T12:00:00Z"
  handovers = [{
    interval      = 1
    interval_type = "daily"
  }]
}

resource "incident_schedule_replica" "test" {
  schedule_id               = incident_schedule.test.id
  replica_provider          = {{ quote .ReplicaProvider }}
  replica_provider_id       = {{ quote .ReplicaProviderID }}
  replica_fallback_user_id  = {{ quote .FallbackUserID }}
  mirror_window_days        = {{ .MirrorWindowDays }}

  sources = [{
    rotation_id = incident_schedule_rotation.test.id
    layer_id    = {{ quote .LayerID }}
  }]
}

data "incident_schedule_replica" "test" {
  schedule_id = incident_schedule.test.id
  id          = incident_schedule_replica.test.id
}
`, struct {
		ReplicaProvider   string
		ReplicaProviderID string
		FallbackUserID    string
		MirrorWindowDays  int
		LayerID           string
	}{
		ReplicaProvider:   replicaProvider,
		ReplicaProviderID: replicaProviderID,
		FallbackUserID:    fallbackUserID,
		MirrorWindowDays:  mirrorWindowDays,
		LayerID:           layerID,
	})
}
