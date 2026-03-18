# Schedule Rotation Versions as Independent Terraform Resources

## Problem

The `incident_schedule` Terraform resource manages schedules as a single monolith — metadata, rotations, and all rotation versions are deeply nested in one resource. This causes:

- **Unreadable diffs**: Changes to a rotation version are buried 3 levels deep in `terraform plan` output
- **Easy to accidentally mutate**: Users modify existing versions instead of creating new ones
- **Hard to copy-paste**: Adding a new version means editing nested arrays rather than duplicating a resource block

Customers (notably Vercel) have asked for rotation versions to be their own Terraform resources, enabling an append-only workflow where adding a person or changing a schedule means copying a resource block and setting a new `effective_from`.

## Design Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Backend changes | New API endpoints | Clean sub-resource CRUD, no race conditions from provider-side read-modify-write |
| New TF resource name | `incident_schedule_rotation_version` | Matches the `versions` key in the internal data model |
| Backward compatibility | Breaking change (v6) | `incident_schedule` becomes metadata-only, `rotations` field removed |
| Migration | Import-based with UUID backfill | Toolbox backfill assigns version UUIDs, users import existing versions into new resources |

## Terraform Interface

### `incident_schedule` (v6, metadata only)

```hcl
resource "incident_schedule" "primary" {
  name     = "Primary On-call"
  timezone = "Europe/London"
  team_ids = [data.incident_team.platform.id]

  holidays_public_config = {
    country_codes = ["GB", "FR"]
  }
}
```

Fields: `id` (computed), `name`, `timezone`, `team_ids` (optional), `holidays_public_config` (optional).

The `rotations` field is removed entirely.

### `incident_schedule_rotation_version` (new)

```hcl
resource "incident_schedule_rotation_version" "emea_v1" {
  schedule_id   = incident_schedule.primary.id
  rotation_id   = "emea"
  rotation_name = "EMEA Rotation"

  handover_start_at = "2024-05-01T12:00:00Z"

  users = [
    data.incident_user.alice.id,
    data.incident_user.bob.id,
  ]

  layers = [{
    id   = "primary"
    name = "Primary"
  }]

  handovers = [{
    interval_type = "weekly"
    interval      = 1
  }]
}

# New version: copy-paste, set effective_from, change what's needed
resource "incident_schedule_rotation_version" "emea_v2" {
  schedule_id   = incident_schedule.primary.id
  rotation_id   = "emea"
  rotation_name = "EMEA Rotation"

  effective_from    = "2024-06-01T12:00:00Z"
  handover_start_at = "2024-05-01T12:00:00Z"

  users = [
    data.incident_user.alice.id,
    data.incident_user.bob.id,
    data.incident_user.charlie.id,
  ]

  layers = [{
    id   = "primary"
    name = "Primary"
  }]

  handovers = [{
    interval_type = "weekly"
    interval      = 1
  }]
}
```

#### Schema

| Attribute | Type | Required | Description |
|-----------|------|----------|-------------|
| `id` | string | computed | Server-generated UUID for this version entry |
| `schedule_id` | string | required | Parent schedule ID |
| `rotation_id` | string | required | User-provided ID grouping versions of the same rotation |
| `rotation_name` | string | required | Display name for the rotation |
| `effective_from` | string | optional | RFC3339 timestamp; absent = the base version |
| `handover_start_at` | string | required | RFC3339 timestamp for handover cadence start |
| `users` | list(string) | required | User IDs on this rotation |
| `layers` | list(object) | required | Layer definitions (`id`, `name`) |
| `handovers` | list(object) | required | Handover rules (`interval`, `interval_type`) |
| `working_intervals` | list(object) | optional | Weekday time restrictions (`start_time`, `end_time`, `weekday`) |
| `scheduling_mode` | string | optional | Scheduling algorithm: `fair` or `sequential`. Defaults to backend default if omitted. |

The `users` field is a flat list of user ID strings in the Terraform schema. The provider converts these to `UserReferencePayloadV2` objects (`{id: "..."}`) when calling the API, matching the existing convention used by the current schedule resource.

## Backend API Endpoints

New public V2 endpoints for managing individual rotation version entries within a schedule's config. These follow the existing path conventions under the public V2 API (alongside `/schedules`, `/schedule_entries`, etc.).

### Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/schedule_rotation_versions` | Create a rotation version |
| `GET` | `/schedule_rotation_versions/{id}` | Show a specific rotation version |
| `PUT` | `/schedule_rotation_versions/{id}` | Update a rotation version |
| `DELETE` | `/schedule_rotation_versions/{id}` | Remove a rotation version |
| `GET` | `/schedule_rotation_versions?schedule_id={id}` | List versions for a schedule |

These are defined as `ServicePublicV2` methods, matching the existing schedules service pattern.

### Data Model Change

Add a `VersionID` field to the `ScheduleRotation` struct in the config JSONB:

```go
type ScheduleRotation struct {
    VersionID        string     `json:"version_id,omitempty"` // NEW — unique ID for this entry
    ID               string     `json:"id"`                   // existing — rotation group ID
    Name             string     `json:"name"`
    EffectiveFrom    *time.Time `json:"effective_from,omitempty"`
    HandoverStartAt  time.Time  `json:"handover_start_at"`
    SchedulingMode   string     `json:"scheduling_mode,omitempty"`
    Handovers        []ScheduleRotationHandover
    UserIDs          []string
    WorkingIntervals []ScheduleRotationWorkingInterval
    Layers           []ScheduleLayer
}
```

The `omitempty` tag on `VersionID` ensures backward compatibility: existing JSONB entries without the field deserialize cleanly as empty string, and the backfill command populates them.

### Behavior

- **Create**: Generates a UUID for `VersionID`, adds the entry to the schedule's config, triggers the normal schedule version snapshot (audit trail). Returns the full version object including `schedule_id`.
- **Show**: Reads the schedule config, finds the entry by `VersionID`, returns it along with `schedule_id`. This enables import by `VersionID` alone.
- **Update**: Finds the entry by `VersionID`, replaces it, writes back. Creates a new schedule version snapshot.
- **Delete**: Removes the entry by `VersionID`. If this was the last version of a rotation, the rotation ceases to exist.
- **List**: Returns all rotation entries for a given schedule, with their `VersionID`s.

### Concurrency

Each write operation (create/update/delete) executes within a single transaction that:

1. Acquires a row-level lock on the schedule via `SELECT ... FOR UPDATE` (matching the existing `SchedulesV2Update` pattern)
2. Reads the current config
3. Modifies the target rotation entry
4. Bumps the config version number
5. Writes back and commits

Each write also increments `Config.Version`, maintaining consistency with the existing optimistic locking used by `SchedulesV2Update`. This ensures that if a dashboard user loaded a schedule before a Terraform apply, their subsequent save gets a version mismatch error rather than silently overwriting Terraform's changes.

Concurrent Terraform operations against the same schedule serialize at the database level — if Terraform applies two `incident_schedule_rotation_version` resources in parallel, the second transaction blocks on `FOR UPDATE` until the first completes, then reads the up-to-date config. No stale reads, no conflicts.

If a conflict does occur (e.g., due to a dashboard update racing with Terraform), the endpoint returns HTTP 409 Conflict. The Terraform provider should implement retry logic (retry up to 3 times with short backoff) on 409 responses, since these are transient.

### Validation

- `effective_from` values must be unique within a rotation group. Two versions with the same `rotation_id` and the same `effective_from` (including both being null) are rejected. This matches the existing backend validation.
- A rotation group may have zero or one version without `effective_from`. There is no requirement for a "base" version — all versions can have explicit `effective_from` dates. (The existing backend/dashboard already permits this.)
- All versions sharing a `rotation_id` must have the same `rotation_name`. The backend validates this on create/update and rejects mismatches. To rename a rotation, update any one version — the backend propagates the name change to all versions of that rotation in the same transaction.

### Import

The `incident_schedule_rotation_version` resource supports import by `VersionID` alone:

```bash
terraform import incident_schedule_rotation_version.emea_v1 <version-uuid>
```

The `ImportState` implementation calls the Show endpoint with the `VersionID`, which returns all fields including `schedule_id` and `rotation_id`, fully reconstructing the state.

## Dashboard Compatibility

The existing `SchedulesV2Update` endpoint (full config replacement) continues to work. The web dashboard uses this.

When the dashboard updates a schedule:
- If rotation entries already have `VersionID` fields, the full config replacement round-trips them — the `VersionID` values are preserved in the JSONB.
- If the dashboard adds a new rotation entry without a `VersionID`, the backend assigns one automatically on write (same as the Create endpoint).
- Terraform state remains consistent because the provider reads state from the API on each `terraform plan`/`apply` — if the dashboard has modified config, Terraform detects the drift via the normal read path.

## Schedule Deletion

When a user deletes the `incident_schedule` resource, the schedule is archived (soft-deleted) on the backend, which implicitly removes all rotation versions. Any `incident_schedule_rotation_version` resources referencing that schedule will fail their next Read with a 404, and Terraform will remove them from state.

To ensure clean ordering, the `incident_schedule_rotation_version` resource should declare an implicit dependency on its `schedule_id` reference (which Terraform infers automatically from the `incident_schedule.primary.id` interpolation). Terraform will delete rotation versions before the schedule.

## Backfill: Toolbox Command

Existing schedules have rotation entries without `VersionID`s. A toolbox backfill populates them so they can be referenced by the new endpoints and imported into Terraform.

### Command

```bash
./toolbox backfill --name=backfill-schedule-rotation-version-ids
```

### Behavior

1. Iterate over all schedules (all organisations)
2. For each rotation entry in the schedule's config JSONB where `VersionID` is empty:
   - Generate a new UUID
   - Assign it as the entry's `VersionID`
3. Write the updated config back within a transaction (using `SELECT ... FOR UPDATE`)
4. Log each schedule updated with the count of IDs assigned

Idempotent — running twice is safe (entries that already have a `VersionID` are skipped).

### Implementation Pattern

```go
package backfill

func init() {
    backfill.Register("backfill-schedule-rotation-version-ids", runBackfillVersionIDs)
}

func runBackfillVersionIDs(ctx context.Context, db *gorm.DB, options backfill.Options) error {
    // 1. Load all schedules with their configs
    // 2. For each schedule, check each rotation entry
    // 3. If VersionID is empty, assign uuid.New().String()
    // 4. Write back within transaction with FOR UPDATE lock
    // 5. Log progress
    return nil
}
```

### Deployment Order

1. Deploy backend with `VersionID` field added to `ScheduleRotation` struct (backward-compatible, `omitempty`)
2. Run backfill: `./toolbox backfill --name=backfill-schedule-rotation-version-ids`
3. Deploy new `/schedule_rotation_versions` endpoints (they can now assume all entries have `VersionID`s; the endpoints also assign UUIDs to any entries that were created between step 1 and 3 without one)
4. Release Terraform provider v6

## Migration Guide (for v6 changelog)

### Steps

1. Update the provider to v6
2. Discover existing rotation version IDs:
   ```bash
   curl -H "Authorization: Bearer $API_KEY" \
     https://api.incident.io/v2/schedule_rotation_versions?schedule_id=SCHEDULE_ID
   ```
3. Write `incident_schedule_rotation_version` resources for each version
4. Import them:
   ```bash
   terraform import incident_schedule_rotation_version.emea_v1 <version-uuid>
   terraform import incident_schedule_rotation_version.emea_v2 <version-uuid>
   ```
5. Remove `rotations` from the `incident_schedule` resource in your `.tf` files
6. Run `terraform plan` — should show no changes

## Edge Cases

- **Rotation name consistency**: All versions sharing a `rotation_id` must have the same `rotation_name`. The backend enforces this. To rename, update any version and the backend propagates.
- **Schedule with no rotations**: Valid — represents a schedule with nobody on-call.
- **Deletion ordering**: Terraform deletes rotation versions before the schedule due to the implicit dependency from `schedule_id`. Within rotation versions of the same schedule, deletion order doesn't matter — each removal is independent. Deleting the last version of a rotation group simply removes the rotation entirely.
- **Between deploy and backfill**: Rotation entries without `VersionID` may exist briefly. The new endpoints handle this gracefully by assigning a UUID on write if one is missing. The Show/List endpoints skip entries without `VersionID` (they're not yet addressable).

## Plan readability improvement

The core UX win — what the `terraform plan` diff looks like:

**Before (v5, nested):**
```diff
~ resource "incident_schedule" "primary" {
    ~ rotations = [
        ~ { versions = [
            ~ { users = [...] → [...] }  # buried 3 levels deep
          ]
        }
      ]
  }
```

**After (v6, flat):**
```diff
+ resource "incident_schedule_rotation_version" "emea_v2" {
    rotation_id = "emea"
    users       = ["alice", "bob", "charlie"]
    ...
  }
```
