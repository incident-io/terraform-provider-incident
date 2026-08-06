package provider

import (
	"context"
	"fmt"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timetypes/timetypes"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/internal/client"
)

var (
	_ resource.Resource                   = &IncidentScheduleRotationBetaResource{}
	_ resource.ResourceWithConfigure      = &IncidentScheduleRotationBetaResource{}
	_ resource.ResourceWithImportState    = &IncidentScheduleRotationBetaResource{}
	_ resource.ResourceWithModifyPlan     = &IncidentScheduleRotationBetaResource{}
	_ resource.ResourceWithValidateConfig = &IncidentScheduleRotationBetaResource{}
)

// The strategies the API accepts for introducing a changed line-up.
var scheduleRotationRollouts = []string{"immediate", "after_current_shift", "after_full_rotation"}

func NewIncidentScheduleRotationBetaResource() resource.Resource {
	return &IncidentScheduleRotationBetaResource{}
}

type IncidentScheduleRotationBetaResource struct {
	client           *client.ClientWithResponses
	terraformVersion string
}

// IncidentScheduleRotationBetaModel is the Terraform state/plan shape for the resource.
type IncidentScheduleRotationBetaModel struct {
	ID                    types.String                                `tfsdk:"id"`
	ScheduleID            types.String                                `tfsdk:"schedule_id"`
	Name                  types.String                                `tfsdk:"name"`
	Users                 []types.String                              `tfsdk:"users"`
	Handovers             []IncidentScheduleRotationBetaHandover      `tfsdk:"handovers"`
	FirstIntervalStartsAt timetypes.RFC3339                           `tfsdk:"first_interval_starts_at"`
	ConcurrentShifts      types.Int64                                 `tfsdk:"concurrent_shifts"`
	WorkingIntervals      []IncidentScheduleRotationBetaWorkingWindow `tfsdk:"working_intervals"`
	Rank                  types.Int64                                 `tfsdk:"rank"`
	SchedulingMode        types.String                                `tfsdk:"scheduling_mode"`
	Rollout               types.String                                `tfsdk:"rollout"`
	EffectiveFrom         timetypes.RFC3339                           `tfsdk:"effective_from"`
}

type IncidentScheduleRotationBetaHandover struct {
	Interval     types.Int64  `tfsdk:"interval"`
	IntervalType types.String `tfsdk:"interval_type"`
}

type IncidentScheduleRotationBetaWorkingWindow struct {
	Weekday   types.String `tfsdk:"weekday"`
	StartTime types.String `tfsdk:"start_time"`
	EndTime   types.String `tfsdk:"end_time"`
}

func (r *IncidentScheduleRotationBetaResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_schedule_rotation_beta"
}

func (r *IncidentScheduleRotationBetaResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `Manage a rotation on an on-call schedule.

A rotation is a group of people who take turns being on call, and the cadence they
hand over on. A schedule can hold several, and each is managed independently — so
changing one never rewrites the others.

Editing a rotation takes effect straight away, unless ` + "`rollout`" + ` says otherwise.

## Changing when shifts hand over

` + "`first_interval_starts_at`" + ` is the point intervals are counted from. With
` + "`handovers`" + ` it fixes the time of day and day of week that shifts change hands —
the dashboard calls it the handover time.

It can't change in the same apply as ` + "`rollout`" + `. Whenever ` + "`rollout`" + ` is set the
rotation keeps the interval start it already runs: an immediate rollout copies that
across so the cadence doesn't move, and a phased one works backwards from the next
handover so the new line-up slots into the rhythm people are already working. Either
way a new ` + "`first_interval_starts_at`" + ` is discarded, so the provider stops the apply
rather than storing a value that won't be kept.

To change both, take it in two applies:

1. Change ` + "`first_interval_starts_at`" + ` on its own, with ` + "`rollout`" + ` unset. The new
   cadence takes effect straight away.
2. Then make the line-up change, with ` + "`rollout`" + ` set.

## Choosing a scheduling mode

` + "`scheduling_mode`" + ` decides who takes the next shift.

` + "`fair`" + ` shares time on call out evenly, tracking how much each person has already
done and giving the next shift to whoever is behind. Someone newly added has done
none, so ` + "`fair`" + ` tends to bring them on call sooner rather than adding them to
the back of the queue.

` + "`sequential`" + ` goes around the list in order, so the next person on call is always
the one after the last.

For an even rotation — everyone on for the same length of time, no working hours,
nobody joining or leaving — the two behave identically. They only diverge once
shifts differ in length, ` + "`working_intervals`" + ` is set, or the list of users
changes. Reach for ` + "`sequential`" + ` when it matters that the running order is
obvious to the people in it.

## Beta, and what happens next

This resource is in beta. Its schema may still change in ways that are not
backwards compatible, so pin the provider version if that matters to you.

The plan is for these resources to become the only way to manage schedules. In
v7.0 they lose the ` + "`_beta`" + ` suffix and ` + "`incident_schedule`" + `, which declares
its rotations inline, is removed. Until then both work and neither is deprecated.
See ` + "`incident_schedule_beta`" + ` for how the two differ and how to migrate.`,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("ScheduleRotationV3", "id"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"schedule_id": schema.StringAttribute{
				Required: true,
				MarkdownDescription: apischema.Docstring("ScheduleRotationV3", "schedule_id") +
					". Changing this replaces the rotation, as a rotation can't move between schedules.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: apischema.Docstring("ScheduleRotationV3", "name"),
			},
			"users": schema.ListAttribute{
				Required:    true,
				ElementType: types.StringType,
				MarkdownDescription: "IDs of the people in the rotation, in the order they take shifts. " +
					"Use the special ID `NOBODY` for a slot with nobody in it, which schedules " +
					"the shift without putting anyone on call until an override covers it.",
			},
			"handovers": schema.ListNestedAttribute{
				Required:            true,
				MarkdownDescription: apischema.Docstring("ScheduleRotationV3", "handovers"),
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"interval": schema.Int64Attribute{
							Required:            true,
							MarkdownDescription: "How many of the interval type to wait between handovers.",
						},
						"interval_type": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: "One of hourly, daily or weekly.",
						},
					},
				},
			},
			// The RFC3339 type rejects a malformed timestamp at plan time, and reconciles
			// "Z" with a zero offset. It stops short of treating two different offsets for
			// one instant as equal, which is part of what anchorToState is for.
			"first_interval_starts_at": schema.StringAttribute{
				Required:   true,
				CustomType: timetypes.RFC3339Type{},
				MarkdownDescription: apischema.Docstring("ScheduleRotationV3", "first_interval_starts_at") +
					". Can't be changed in the same apply as `rollout` — see the resource " +
					"description for why, and what to do instead.",
			},
			"concurrent_shifts": schema.Int64Attribute{
				Optional: true,
				Computed: true,
				Default:  int64default.StaticInt64(1),
				MarkdownDescription: apischema.Docstring("ScheduleRotationV3", "concurrent_shifts") +
					". Defaults to 1, one person on call at a time. Reducing this stops scheduling the last of them, and stops any overrides on those shifts from applying.",
			},
			"working_intervals": schema.ListNestedAttribute{
				Optional: true,
				MarkdownDescription: "If set, restricts on-call to these weekday intervals. " +
					"Omit it to keep the rotation on call around the clock. An empty list is " +
					"not valid, and is rejected at plan time — it would leave a rotation " +
					"nobody is ever on call for.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"weekday": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: "Day of the week, lowercase, e.g. monday.",
						},
						"start_time": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: "Time of day this window opens, as HH:MM.",
						},
						"end_time": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: "Time of day this window closes, as HH:MM.",
						},
					},
				},
			},
			// Optional but not Computed, so leaving it out means Terraform doesn't manage
			// the order. Computed would adopt whatever rank it found and then re-assert
			// it, fighting anyone who reorders the schedule in the dashboard.
			"rank": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: apischema.Docstring("ScheduleRotationV3", "rank"),
			},
			// Optional but not Computed, for the same reason as rank: a rotation that has
			// never stated a mode has none to read back. The API leaves scheduling_mode out
			// of the response entirely in that case, so Computed couldn't be satisfied —
			// the mode it falls back to is a domain default the wire never mentions.
			"scheduling_mode": schema.StringAttribute{
				Optional: true,
				MarkdownDescription: apischema.Docstring("ScheduleRotationV3", "scheduling_mode") +
					fmt.Sprintf(". One of %s. Omit it to leave us to pick. For an even rotation "+
						"the two behave identically — see the resource description for when they diverge.",
						strings.Join(scheduleRotationSchedulingModes, ", ")),
			},
			// Only read on an edit, and never returned to us, so it stays exactly what
			// the config says rather than being adopted from the rotation.
			"rollout": schema.StringAttribute{
				Optional: true,
				MarkdownDescription: "How a change to this rotation is introduced: `immediate` " +
					"replaces the line-up now and can change who is on call this minute, " +
					"`after_current_shift` lets the shift on call finish first, and " +
					"`after_full_rotation` waits for everyone to have taken a turn. Leave it " +
					"out to replace the line-up straight away. Creating a rotation ignores " +
					"it, as there's no shift to protect yet.",
			},
			"effective_from": schema.StringAttribute{
				Computed:            true,
				CustomType:          timetypes.RFC3339Type{},
				MarkdownDescription: apischema.Docstring("ScheduleRotationV3", "effective_from"),
			},
		},
	}
}

func (r *IncidentScheduleRotationBetaResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	data, ok := req.ProviderData.(*IncidentProviderData)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected provider data",
			fmt.Sprintf("expected *IncidentProviderData, got %T. This is a provider bug.", req.ProviderData),
		)
		return
	}
	r.client = data.Client
	r.terraformVersion = data.TerraformVersion
}

// handoverIntervalLimits is the largest repeat allowed for each interval type: a weekly
// rotation can hand over every fourth week, but not every fifth. These bounds live in
// the domain rather than the API schema, so unlike the enums below they're a copy — a
// type the schema gains without an entry here just goes unchecked at plan time.
var handoverIntervalLimits = map[string]int64{
	"hourly": 23,
	"daily":  14,
	"weekly": 4,
}

// maxConcurrentShifts mirrors the API's ceiling. Also not in the schema we vendor.
const maxConcurrentShifts = 20

var (
	handoverIntervalTypes           = enumValues("ScheduleRotationHandoverV2", "interval_type")
	workingIntervalWeekdays         = enumValues("ScheduleRotationWorkingIntervalCreatePayloadV2", "weekday")
	scheduleRotationSchedulingModes = enumValues("ScheduleRotationV3", "scheduling_mode")
)

// clockTimePattern is the time of day the API accepts: two digits each, 24-hour clock.
var clockTimePattern = regexp.MustCompile(`^(0[0-9]|1[0-9]|2[0-3]):[0-5][0-9]$`)

// ValidateConfig checks what the schema can't, so a bad config fails at plan time
// rather than part-way through an apply. Each check mirrors one the API makes, and only
// ever rejects what the API would: being stricter here would fail a config that applies
// fine, which is worse than leaving the 422 where it already was.
//
// It reads attributes one at a time rather than decoding IncidentScheduleRotationBetaModel,
// because the model holds working_intervals as a slice, which can't represent a
// list that isn't known yet — decoding a config built from another resource's
// output would fail here instead of validating.
//
// Every check is judged on its own diagnostics rather than the response's, so one bad
// attribute doesn't hide the next: a config with two mistakes in it should report both,
// not send someone round the loop twice. For the same reason each value is guarded
// before it's judged — unknown is not null, and treating it as a value would reject a
// config that's fine.
func (r *IncidentScheduleRotationBetaResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	r.validateName(ctx, req, resp)
	r.validateUsers(ctx, req, resp)
	r.validateHandovers(ctx, req, resp)
	r.validateWorkingIntervals(ctx, req, resp)
	r.validateConcurrentShifts(ctx, req, resp)
	r.validateRank(ctx, req, resp)
	r.validateRollout(ctx, req, resp)
	r.validateSchedulingMode(ctx, req, resp)
}

// validateName rejects a blank name. Required only makes Terraform insist the attribute
// is set, and the API has no minimum length, so an empty string gets all the way to the
// domain before anything objects.
func (r *IncidentScheduleRotationBetaResource) validateName(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	namePath := path.Root("name")

	var name types.String
	diags := req.Config.GetAttribute(ctx, namePath, &name)
	resp.Diagnostics.Append(diags...)
	if diags.HasError() || name.IsNull() || name.IsUnknown() {
		return
	}

	// Only an empty string: the domain compares against "", so a name of spaces is
	// something it accepts and we've no business rejecting.
	if name.ValueString() == "" {
		resp.Diagnostics.AddAttributeError(
			namePath,
			"Empty name",
			"Give the rotation a name. It's how the rotation is identified on the schedule, and how a dashboard-authored one is matched on import.",
		)
	}
}

// validateUsers rejects a rotation with nobody in it. This is the only place the mistake
// surfaces: the API accepts an empty list, and the renderer then schedules "NOBODY"
// rather than failing, so the rotation applies cleanly and pages no one.
func (r *IncidentScheduleRotationBetaResource) validateUsers(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	usersPath := path.Root("users")

	var users types.List
	diags := req.Config.GetAttribute(ctx, usersPath, &users)
	resp.Diagnostics.Append(diags...)
	if diags.HasError() || users.IsNull() || users.IsUnknown() {
		return
	}

	if len(users.Elements()) == 0 {
		resp.Diagnostics.AddAttributeError(
			usersPath,
			"Empty users",
			"A rotation needs at least one entry. For a rotation that schedules shifts without "+
				"putting anyone on call — a shadow rotation covered by overrides — use the special "+
				"ID NOBODY rather than an empty list. Remove the rotation instead if it shouldn't "+
				"exist at all.",
		)
	}
}

// validateHandovers checks the cadence shifts change hands on: at least one, a recognised
// interval type, and a repeat the domain allows for that type.
func (r *IncidentScheduleRotationBetaResource) validateHandovers(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	handoversPath := path.Root("handovers")

	var handovers types.List
	diags := req.Config.GetAttribute(ctx, handoversPath, &handovers)
	resp.Diagnostics.Append(diags...)
	if diags.HasError() || handovers.IsNull() || handovers.IsUnknown() {
		return
	}

	if len(handovers.Elements()) == 0 {
		resp.Diagnostics.AddAttributeError(
			handoversPath,
			"Empty handovers",
			"Set at least one handover, so shifts have a cadence to change hands on.",
		)
		return
	}

	for i, element := range handovers.Elements() {
		elementPath := handoversPath.AtListIndex(i)

		object, ok := element.(types.Object)
		if !ok || object.IsNull() || object.IsUnknown() {
			continue
		}
		attributes := object.Attributes()

		intervalType, typeKnown := knownString(attributes["interval_type"])
		if typeKnown && !slices.Contains(handoverIntervalTypes, intervalType) {
			resp.Diagnostics.AddAttributeError(
				elementPath.AtName("interval_type"),
				"Invalid interval_type",
				fmt.Sprintf("%q isn't a handover interval type. Use one of %s.",
					intervalType, strings.Join(handoverIntervalTypes, ", ")),
			)
			continue
		}

		interval, intervalKnown := knownInt64(attributes["interval"])
		limit, haveLimit := handoverIntervalLimits[intervalType]
		if !typeKnown || !intervalKnown || !haveLimit {
			continue
		}

		if interval < 1 || interval > limit {
			resp.Diagnostics.AddAttributeError(
				elementPath.AtName("interval"),
				"Invalid interval",
				fmt.Sprintf("A %s handover repeats every 1 to %d, not every %d.", intervalType, limit, interval),
			)
		}
	}
}

// validateWorkingIntervals checks the windows that restrict when the rotation is on call.
//
// Omitting working_intervals is how you say "on call around the clock", so an empty list
// is someone reaching for that and getting a rotation nobody is ever on call for.
func (r *IncidentScheduleRotationBetaResource) validateWorkingIntervals(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	intervalsPath := path.Root("working_intervals")

	var intervals types.List
	diags := req.Config.GetAttribute(ctx, intervalsPath, &intervals)
	resp.Diagnostics.Append(diags...)
	if diags.HasError() || intervals.IsNull() || intervals.IsUnknown() {
		return
	}

	if len(intervals.Elements()) == 0 {
		resp.Diagnostics.AddAttributeError(
			intervalsPath,
			"Empty working_intervals",
			"Set at least one interval, or remove the attribute to keep the rotation on call around the clock.",
		)
		return
	}

	// The windows we could read in full, as minutes since midnight, for the overlap check
	// below. One we couldn't read is either reported here or isn't knowable yet, and takes
	// no part in it.
	type window struct {
		path    path.Path
		weekday string
		start   int
		end     int
	}
	var windows []window

	for i, element := range intervals.Elements() {
		elementPath := intervalsPath.AtListIndex(i)

		object, ok := element.(types.Object)
		if !ok || object.IsNull() || object.IsUnknown() {
			continue
		}
		attributes := object.Attributes()

		// Reads one of the two times, reporting a malformed one against its own path.
		readTime := func(name string) (int, bool) {
			value, ok := knownString(attributes[name])
			if !ok {
				return 0, false
			}

			minutes, ok := clockTimeMinutes(value)
			if !ok {
				resp.Diagnostics.AddAttributeError(
					elementPath.AtName(name),
					fmt.Sprintf("Invalid %s", name),
					fmt.Sprintf("%q isn't a time of day. Use 24-hour HH:MM, like 09:00.", value),
				)
			}

			return minutes, ok
		}

		weekday, weekdayKnown := knownString(attributes["weekday"])
		if weekdayKnown && !slices.Contains(workingIntervalWeekdays, weekday) {
			resp.Diagnostics.AddAttributeError(
				elementPath.AtName("weekday"),
				"Invalid weekday",
				fmt.Sprintf("%q isn't a weekday. Use a lowercase name like monday.", weekday),
			)
			weekdayKnown = false
		}

		start, startKnown := readTime("start_time")
		end, endKnown := readTime("end_time")

		if weekdayKnown && startKnown && endKnown {
			windows = append(windows, window{path: elementPath, weekday: weekday, start: start, end: end})
		}
	}

	// Only windows that stay within their weekday are compared. One whose end time is
	// before its start runs past midnight and belongs partly to the next day, and the
	// API's own overlap check doesn't spot a clash involving those — so judging them
	// here would reject a config it accepts.
	for i, this := range windows {
		if this.end < this.start {
			continue
		}

		for _, that := range windows[:i] {
			if that.weekday != this.weekday || that.end < that.start {
				continue
			}

			if this.start < that.end && that.start < this.end {
				resp.Diagnostics.AddAttributeError(
					this.path,
					"Overlapping working_intervals",
					fmt.Sprintf("This interval overlaps another %s interval. Merge them into a single window.", this.weekday),
				)
				break
			}
		}
	}
}

// validateConcurrentShifts mirrors the bounds on the API. The attribute is required, so
// the only way to get it wrong is a number out of range.
func (r *IncidentScheduleRotationBetaResource) validateConcurrentShifts(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	shiftsPath := path.Root("concurrent_shifts")

	var shifts types.Int64
	diags := req.Config.GetAttribute(ctx, shiftsPath, &shifts)
	resp.Diagnostics.Append(diags...)
	if diags.HasError() || shifts.IsNull() || shifts.IsUnknown() {
		return
	}

	if value := shifts.ValueInt64(); value < 1 || value > maxConcurrentShifts {
		resp.Diagnostics.AddAttributeError(
			shiftsPath,
			"Invalid concurrent_shifts",
			fmt.Sprintf("A rotation runs between 1 and %d shifts at the same time, not %d.", maxConcurrentShifts, value),
		)
	}
}

// validateRank mirrors the API's lower bound. Rank counts from one, and zero is how the
// config records a rotation that has never been ordered, so it isn't something to ask for
// — leave the attribute out instead.
func (r *IncidentScheduleRotationBetaResource) validateRank(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	rankPath := path.Root("rank")

	var rank types.Int64
	diags := req.Config.GetAttribute(ctx, rankPath, &rank)
	resp.Diagnostics.Append(diags...)
	if diags.HasError() || rank.IsNull() || rank.IsUnknown() {
		return
	}

	if value := rank.ValueInt64(); value < 1 {
		resp.Diagnostics.AddAttributeError(
			rankPath,
			"Invalid rank",
			fmt.Sprintf("Rank counts from 1, so %d isn't a position. Remove the attribute to leave the rotation unordered.", value),
		)
	}
}

// validateRollout rejects a strategy the API doesn't offer.
func (r *IncidentScheduleRotationBetaResource) validateRollout(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	rolloutPath := path.Root("rollout")

	var rollout types.String
	diags := req.Config.GetAttribute(ctx, rolloutPath, &rollout)
	resp.Diagnostics.Append(diags...)
	if diags.HasError() || rollout.IsNull() || rollout.IsUnknown() {
		return
	}

	if !slices.Contains(scheduleRotationRollouts, rollout.ValueString()) {
		resp.Diagnostics.AddAttributeError(
			rolloutPath,
			"Unknown rollout",
			fmt.Sprintf("Set one of %s, or remove the attribute to replace the line-up straight away.",
				strings.Join(scheduleRotationRollouts, ", ")),
		)
	}
}

// validateSchedulingMode rejects an allocation strategy the API doesn't offer.
func (r *IncidentScheduleRotationBetaResource) validateSchedulingMode(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	modePath := path.Root("scheduling_mode")

	var mode types.String
	diags := req.Config.GetAttribute(ctx, modePath, &mode)
	resp.Diagnostics.Append(diags...)
	if diags.HasError() || mode.IsNull() || mode.IsUnknown() {
		return
	}

	if !slices.Contains(scheduleRotationSchedulingModes, mode.ValueString()) {
		resp.Diagnostics.AddAttributeError(
			modePath,
			"Unknown scheduling_mode",
			fmt.Sprintf("Set one of %s, or remove the attribute to leave us to pick.",
				strings.Join(scheduleRotationSchedulingModes, ", ")),
		)
	}
}

// clockTimeMinutes parses an HH:MM time of day into minutes since midnight.
func clockTimeMinutes(value string) (int, bool) {
	if !clockTimePattern.MatchString(value) {
		return 0, false
	}

	hours, err := strconv.Atoi(value[:2])
	if err != nil {
		return 0, false
	}
	minutes, err := strconv.Atoi(value[3:])
	if err != nil {
		return 0, false
	}

	return hours*60 + minutes, true
}

// ModifyPlan runs the two plan-time checks a rotation change deserves: what it costs
// the overrides sitting on the rotation, and when it actually takes over.
//
// Overrides come first so that a change which both strands overrides and can't be
// planned still says what it would cost, rather than reporting only the second
// problem and leaving the first to be discovered after the apply.
func (r *IncidentScheduleRotationBetaResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	r.warnStrandedOverrides(ctx, req, resp)
	r.warnSupersededChange(ctx, req, resp)
	r.planEffectiveFrom(ctx, req, resp)
}

// warnSupersededChange says so when an edit lands on a change that was already
// scheduled — a rotation holds at most one. Easy to walk into after an import, since we
// report the scheduled shape rather than the line-up on call.
func (r *IncidentScheduleRotationBetaResource) warnSupersededChange(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	// A create has nothing scheduled, and a destroy or replace takes the whole rotation
	// with its scheduled change — the override warning is what matters there.
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() || len(resp.RequiresReplace) > 0 {
		return
	}

	// A plan that changes nothing writes nothing, so the scheduled change survives.
	if req.Plan.Raw.Equal(req.State.Raw) {
		return
	}

	var state, plan IncidentScheduleRotationBetaModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() || state.EffectiveFrom.IsNull() || state.EffectiveFrom.IsUnknown() {
		return
	}

	effectiveFrom, diags := state.EffectiveFrom.ValueRFC3339Time()
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() || !effectiveFrom.After(time.Now()) {
		return
	}

	name, scheduledFor := state.Name.ValueString(), effectiveFrom.UTC().Format(time.RFC3339)

	// A rollout picks its own moment and discards the scheduled one; without a rollout
	// the moment stays and only what happens at it changes. Same condition Update sends
	// the rollout on, since a rollout is only phased in when there's a line-up to phase.
	if plan.Rollout.IsNull() || !rotationLineUpDiffers(plan, state) {
		resp.Diagnostics.AddWarning(
			"This edits a change that is already scheduled",
			fmt.Sprintf("Rotation %q has a change scheduled for %s, and this rewrites that "+
				"change rather than who is on call now. Set rollout to immediate to change "+
				"the current line-up instead.", name, scheduledFor),
		)
		return
	}

	resp.Diagnostics.AddWarning(
		"A scheduled change to this rotation will be replaced",
		fmt.Sprintf("Rotation %q has a change scheduled for %s. Rolling this edit out "+
			"replaces it, so that change won't happen — a rotation can only have one "+
			"scheduled at a time.", name, scheduledFor),
	)
}

// planEffectiveFrom works out when a change to this rotation takes over, so the plan
// says so before anyone applies it.
//
// Without a rollout an edit replaces the line-up on the spot and the rotation keeps
// the effective_from it already has, which is what Terraform assumes anyway. With
// one, the moment comes from the rotation's own cadence, so we ask the API for it
// and hand the answer back on apply — which refuses the write if the schedule has
// moved on since. A plan someone sat on then fails rather than quietly landing the
// change at a time they never saw.
func (r *IncidentScheduleRotationBetaResource) planEffectiveFrom(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	// A rotation being destroyed, created or replaced has no shift to protect, so it
	// takes effect immediately whatever the config asked for.
	if r.client == nil || req.Plan.Raw.IsNull() || req.State.Raw.IsNull() || len(resp.RequiresReplace) > 0 {
		return
	}

	var rollout types.String
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("rollout"), &rollout)...)
	if resp.Diagnostics.HasError() || rollout.IsNull() || rollout.IsUnknown() {
		return
	}

	// Nothing to preview, but the rotation's shape isn't settled either, so leave the
	// moment for apply to fill in rather than promising the one it has today.
	if !rotationInputsKnown(req.Plan.Raw) {
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx,
			path.Root("effective_from"), timetypes.NewRFC3339Unknown())...)
		return
	}

	var plan, state IncidentScheduleRotationBetaModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() || !rotationLineUpDiffers(plan, state) {
		return
	}

	startsAt, diags := plan.FirstIntervalStartsAt.ValueRFC3339Time()
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	strategy := client.SchedulesPreviewRotationRolloutPayloadV3Rollout(rollout.ValueString())
	result, err := r.client.SchedulesV3PreviewRotationRolloutWithResponse(ctx,
		state.ScheduleID.ValueString(), state.ID.ValueString(),
		client.SchedulesV3PreviewRotationRolloutJSONRequestBody{
			From:     time.Now().UTC(),
			Rollout:  &strategy,
			Rotation: rotationUpdatePayload(plan, startsAt),
		})
	if err != nil {
		resp.Diagnostics.AddError("Unable to preview schedule rotation rollout", err.Error())
		return
	}
	if result.JSON200 == nil {
		resp.Diagnostics.AddError("Unable to preview schedule rotation rollout",
			fmt.Sprintf("unexpected response: %s", result.Status()))
		return
	}

	previewed := result.JSON200.Rotation

	// Any rollout keeps the interval start the rotation already runs — immediate copies
	// it across, a phased one works back from the next handover — so a first_interval_
	// starts_at moved in the same breath is dropped. It's a required attribute, which
	// Terraform expects an apply to store exactly as planned, so left alone this
	// surfaces as an apply that blames the provider for a bug. Say what's actually
	// happening, before anything is written.
	//
	// Read from the preview rather than assumed, so this stops firing by itself if the
	// API ever starts honouring both in one write.
	timezone := r.scheduleTimezone(ctx, state.ScheduleID.ValueString())
	if !anchorToState(previewed, plan.FirstIntervalStartsAt, timezone).Equal(plan.FirstIntervalStartsAt) {
		resp.Diagnostics.AddAttributeError(
			path.Root("first_interval_starts_at"),
			"Can't change first_interval_starts_at and rollout together",
			"Whenever rollout is set, the rotation keeps the interval start it already runs, "+
				"so the value set here would be discarded rather than stored.\n\n"+
				"Apply the first_interval_starts_at change on its own with rollout unset, then "+
				"make the line-up change with rollout set. Or drop rollout from this change, if "+
				"replacing the line-up straight away is acceptable.\n\n"+
				"See \"Changing when shifts hand over\" in the incident_schedule_rotation_beta "+
				"documentation for why.",
		)
		return
	}

	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("effective_from"),
		timetypes.NewRFC3339TimePointerValue(previewed.EffectiveFrom))...)
}

// overrideLookupPageSize is the list endpoint's maximum, so a rotation's overrides
// come back in as few round trips as we can manage.
const overrideLookupPageSize = 250

// warnStrandedOverrides warns when a planned change takes on-call layers away from the
// rotation, because the overrides sitting on those layers stop applying with them. The
// rows aren't archived, but nothing renders an override once its layer is gone, and
// adding the shift back mints a new layer rather than reviving the old one — so the
// effect is permanent even though nothing is deleted.
//
// Only a change that actually loses layers asks the API anything. Layer IDs survive
// every other edit — a rename, a different set of users, a new handover cadence — so
// their overrides keep applying, and planning an unchanged rotation costs no calls.
func (r *IncidentScheduleRotationBetaResource) warnStrandedOverrides(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	// A rotation being created has no overrides yet, and Configure hasn't run when
	// Terraform is only validating the configuration.
	if req.State.Raw.IsNull() || r.client == nil {
		return
	}

	var state IncidentScheduleRotationBetaModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Destroying a rotation drops every shape of it from the schedule, so all of its
	// overrides go — not just those on a layer being trimmed.
	if req.Plan.Raw.IsNull() {
		r.warnLostOverrides(ctx, resp, state, nil, nil, func(count string) string {
			return fmt.Sprintf("Destroying rotation %q stops %s on it from applying. "+
				"Recreate the overrides you still need on another rotation.",
				state.Name.ValueString(), count)
		})
		return
	}

	var plan IncidentScheduleRotationBetaModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Moving a rotation between schedules replaces it, and the replacement is a new
	// rotation with new layers, so the overrides on this one are left behind.
	if !plan.ScheduleID.IsUnknown() && plan.ScheduleID.ValueString() != state.ScheduleID.ValueString() {
		scheduleIDPath := path.Root("schedule_id")
		r.warnLostOverrides(ctx, resp, state, nil, &scheduleIDPath, func(count string) string {
			return fmt.Sprintf("Moving rotation %q to another schedule replaces it with a new "+
				"rotation carrying new on-call layers, which stops %s on it from applying. "+
				"Recreate the overrides you still need on the new rotation once it exists.",
				state.Name.ValueString(), count)
		})
		return
	}

	// An edit keeps the layers it already had and drops the trailing ones, so a lower
	// shift count is the only change that loses any. State is the cheap gate here: it
	// costs nothing to read, and settles most plans without calling the API at all.
	planned := plan.ConcurrentShifts.ValueInt64()
	if plan.ConcurrentShifts.IsUnknown() || planned >= state.ConcurrentShifts.ValueInt64() {
		return
	}

	// Which layers go, and what they're called, isn't in state — a rotation only
	// carries its shift count — so ask for the rotation itself.
	result, err := r.client.SchedulesV3ShowRotationWithResponse(ctx,
		state.ScheduleID.ValueString(), state.ID.ValueString())
	if isNotFound(err) {
		return
	}
	if err != nil || result.JSON200 == nil {
		r.warnCheckSkipped(resp, err, result)
		return
	}

	layers := result.JSON200.Rotation.Layers
	if planned >= int64(len(layers)) {
		return
	}

	// An id-less layer can't be matched against an override, so it contributes a name
	// to the message but nothing to the filter — which counts nothing rather than
	// everything, and so can't invent a warning.
	dropped := layers[planned:]
	droppedIDs := map[string]bool{}
	names := make([]string, 0, len(dropped))
	for _, layer := range dropped {
		if layer.Id != nil {
			droppedIDs[*layer.Id] = true
		}
		names = append(names, fmt.Sprintf("%q", lo.FromPtr(layer.Name)))
	}

	shiftsPath := path.Root("concurrent_shifts")
	r.warnLostOverrides(ctx, resp, state, droppedIDs, &shiftsPath, func(count string) string {
		return fmt.Sprintf("Reducing concurrent shifts from %d to %d on rotation %q means %s "+
			"will no longer have a shift to apply to, and will stop having any effect on the "+
			"schedule. This removes shift(s) %s. Review your overrides after this change to "+
			"make sure the right people are on call — recreating the ones you still need on a "+
			"remaining shift, as adding the shift back later creates a new one rather than "+
			"restoring this.",
			len(layers), planned, state.Name.ValueString(), count, strings.Join(names, ", "))
	})
}

// warnLostOverrides counts the overrides the change costs and, when there are any,
// attaches detail as a plan warning. attribute names the attribute at fault, or is nil
// when the whole resource is (a destroy has no plan to point into).
func (r *IncidentScheduleRotationBetaResource) warnLostOverrides(
	ctx context.Context,
	resp *resource.ModifyPlanResponse,
	state IncidentScheduleRotationBetaModel,
	layerIDs map[string]bool,
	attribute *path.Path,
	detail func(count string) string,
) {
	count, err := r.countAffectedOverrides(ctx,
		state.ScheduleID.ValueString(), state.ID.ValueString(), layerIDs)
	// The schedule or rotation already being gone is a destroy that has nothing left
	// to warn about, rather than a failure worth reporting.
	if isNotFound(err) {
		return
	}
	if err != nil {
		r.warnCheckSkipped(resp, err, nil)
		return
	}
	if count == 0 {
		return
	}

	described := describeOverrideCount(count)
	summary := fmt.Sprintf("%s will stop applying", described)
	if attribute == nil {
		resp.Diagnostics.AddWarning(summary, detail(described))
		return
	}

	resp.Diagnostics.AddAttributeWarning(*attribute, summary, detail(described))
}

// countAffectedOverrides counts the rotation's overrides that haven't finished yet,
// which are the ones somebody would notice losing — a past override has already had
// whatever effect it was going to have. layerIDs narrows it to overrides on those
// layers; a nil map counts the whole rotation.
func (r *IncidentScheduleRotationBetaResource) countAffectedOverrides(
	ctx context.Context, scheduleID, rotationID string, layerIDs map[string]bool,
) (int, error) {
	var (
		after *string
		count int
		now   = time.Now()
	)

	for {
		result, err := r.client.SchedulesV2ListOverridesWithResponse(ctx, &client.SchedulesV2ListOverridesParams{
			ScheduleId: scheduleID,
			RotationId: lo.ToPtr(rotationID),
			PageSize:   lo.ToPtr(int64(overrideLookupPageSize)),
			After:      after,
		})
		if err != nil {
			return 0, err
		}
		if result.JSON200 == nil {
			return 0, fmt.Errorf("unexpected response: %s", result.Status())
		}

		for _, override := range result.JSON200.Overrides {
			if layerIDs != nil && !layerIDs[override.LayerId] {
				continue
			}
			if override.EndAt.After(now) {
				count++
			}
		}

		// The endpoint only hands back a cursor while pages are full, so this
		// terminates on the last page.
		if result.JSON200.PaginationMeta == nil || result.JSON200.PaginationMeta.After == nil {
			break
		}
		after = result.JSON200.PaginationMeta.After
	}

	return count, nil
}

// warnCheckSkipped says the override check didn't run. It's a warning rather than an
// error because the apply doesn't depend on it: failing the plan would turn a broken
// advisory lookup into an outage for anyone editing a schedule.
func (r *IncidentScheduleRotationBetaResource) warnCheckSkipped(
	resp *resource.ModifyPlanResponse, err error, result *client.SchedulesV3ShowRotationResponse,
) {
	detail := "unknown error"
	switch {
	case err != nil:
		detail = err.Error()
	case result != nil:
		detail = fmt.Sprintf("unexpected response: %s", result.Status())
	}

	resp.Diagnostics.AddWarning(
		"Couldn't check for affected schedule overrides",
		fmt.Sprintf("This change may stop active or upcoming overrides on the rotation from "+
			"applying, but they couldn't be counted: %s. The apply itself is unaffected.", detail),
	)
}

// describeOverrideCount keeps the noun agreeing with the count, so a warning reads as
// English rather than "1 overrides".
func describeOverrideCount(count int) string {
	if count == 1 {
		return "1 active or upcoming override"
	}

	return fmt.Sprintf("%d active or upcoming overrides", count)
}

func (r *IncidentScheduleRotationBetaResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data IncidentScheduleRotationBetaModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	startsAt, diags := data.FirstIntervalStartsAt.ValueRFC3339Time()
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	payload := client.ScheduleRotationCreatePayloadV3{
		Name:                  data.Name.ValueString(),
		Users:                 toUserReferences(data.Users),
		Handovers:             toHandoversPayload(data.Handovers),
		FirstIntervalStartsAt: startsAt,
		ConcurrentShifts:      data.ConcurrentShifts.ValueInt64Pointer(),
		WorkingIntervals:      toWorkingIntervalsPayload(data.WorkingIntervals),
		Rank:                  data.Rank.ValueInt64Pointer(),
	}

	// Unknown until apply when the config leaves it out, since it's Computed. Omitting
	// it is what lets the server pick.
	if !data.SchedulingMode.IsNull() && !data.SchedulingMode.IsUnknown() {
		mode := client.ScheduleRotationCreatePayloadV3SchedulingMode(data.SchedulingMode.ValueString())
		payload.SchedulingMode = &mode
	}

	result, err := r.client.SchedulesV3CreateRotationWithResponse(ctx, data.ScheduleID.ValueString(),
		client.SchedulesV3CreateRotationJSONRequestBody{Rotation: payload})
	if err != nil {
		resp.Diagnostics.AddError("Unable to create schedule rotation", err.Error())
		return
	}
	if result.JSON201 == nil {
		resp.Diagnostics.AddError("Unable to create schedule rotation",
			fmt.Sprintf("unexpected response: %s", result.Status()))
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, incidentScheduleRotationBetaFromAPI(result.JSON201.Rotation, data,
		r.scheduleTimezone(ctx, data.ScheduleID.ValueString())))...)
}

func (r *IncidentScheduleRotationBetaResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data IncidentScheduleRotationBetaModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.SchedulesV3ShowRotationWithResponse(ctx,
		data.ScheduleID.ValueString(), data.ID.ValueString())
	if isNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to read schedule rotation", err.Error())
		return
	}
	// The client turns any non-2xx into an error, so a missing body is an unexpected
	// success response rather than a deleted rotation. Dropping it from state would
	// make the next apply create a second rotation, so fail instead.
	if result.JSON200 == nil {
		resp.Diagnostics.AddError("Unable to read schedule rotation",
			fmt.Sprintf("unexpected response: %s", result.Status()))
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, incidentScheduleRotationBetaFromAPI(result.JSON200.Rotation, data,
		r.scheduleTimezone(ctx, data.ScheduleID.ValueString())))...)
}

func (r *IncidentScheduleRotationBetaResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state IncidentScheduleRotationBetaModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	startsAt, diags := plan.FirstIntervalStartsAt.ValueRFC3339Time()
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := client.SchedulesV3UpdateRotationJSONRequestBody{
		Rotation: rotationUpdatePayload(plan, startsAt),
	}

	// Only phase in a change there is one to phase: an edit that leaves the line-up
	// alone has nothing to protect the current shift from, and asking for a moment
	// we never previewed would fail the stale-plan check for no reason.
	if !plan.Rollout.IsNull() && rotationLineUpDiffers(plan, state) {
		strategy := client.SchedulesUpdateRotationPayloadV3Rollout(plan.Rollout.ValueString())
		body.Rollout = &strategy

		// The moment ModifyPlan showed. The API rejects the write if it now works out
		// differently, so a change can't land at a time nobody saw in the plan.
		//
		// Deliberately without the `from` the payload also accepts. Pinning the
		// plan-time moment and replaying it here looks like the natural pairing, but
		// doing so caused stale-plan rejections that omitting it solves — so the
		// server works from apply-time now, and the guard below is what catches a
		// plan that has been sat on.
		if !plan.EffectiveFrom.IsNull() && !plan.EffectiveFrom.IsUnknown() {
			expected, diags := plan.EffectiveFrom.ValueRFC3339Time()
			resp.Diagnostics.Append(diags...)
			if resp.Diagnostics.HasError() {
				return
			}
			body.ExpectedEffectiveFrom = &expected
		}
	}

	result, err := r.client.SchedulesV3UpdateRotationWithResponse(ctx,
		state.ScheduleID.ValueString(), state.ID.ValueString(), body)
	if err != nil {
		resp.Diagnostics.AddError("Unable to update schedule rotation", err.Error())
		return
	}
	if result.JSON200 == nil {
		resp.Diagnostics.AddError("Unable to update schedule rotation",
			fmt.Sprintf("unexpected response: %s", result.Status()))
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, incidentScheduleRotationBetaFromAPI(result.JSON200.Rotation, plan,
		r.scheduleTimezone(ctx, state.ScheduleID.ValueString())))...)
}

func (r *IncidentScheduleRotationBetaResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data IncidentScheduleRotationBetaModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.client.SchedulesV3DestroyRotationWithResponse(ctx,
		data.ScheduleID.ValueString(), data.ID.ValueString())
	if err != nil && !isNotFound(err) {
		resp.Diagnostics.AddError("Unable to delete schedule rotation", err.Error())
	}
}

// ImportState takes "<schedule_id>:<rotation_id>", since a rotation is only
// addressable through the schedule that holds it.
func (r *IncidentScheduleRotationBetaResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	scheduleID, rotationID, found := strings.Cut(req.ID, ":")
	if !found || scheduleID == "" || rotationID == "" {
		resp.Diagnostics.AddError(
			"Unexpected import identifier",
			fmt.Sprintf("expected <schedule_id>:<rotation_id>, got %q.", req.ID),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("schedule_id"), scheduleID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), rotationID)...)
}

func toUserReferences(users []types.String) []client.UserReferencePayloadV2 {
	references := make([]client.UserReferencePayloadV2, len(users))
	for i, user := range users {
		references[i] = client.UserReferencePayloadV2{Id: user.ValueStringPointer()}
	}
	return references
}

func toHandoversPayload(handovers []IncidentScheduleRotationBetaHandover) []client.ScheduleRotationHandoverV2 {
	payload := make([]client.ScheduleRotationHandoverV2, len(handovers))
	for i, handover := range handovers {
		payload[i] = client.ScheduleRotationHandoverV2{
			Interval:     handover.Interval.ValueInt64(),
			IntervalType: client.ScheduleRotationHandoverV2IntervalType(handover.IntervalType.ValueString()),
		}
	}
	return payload
}

// toWorkingIntervalsPayload returns an empty list rather than nil when the
// attribute is unset, because omitting it tells the API to leave the current
// windows alone — which would strand a rotation on hours removed from the config.
func toWorkingIntervalsPayload(windows []IncidentScheduleRotationBetaWorkingWindow) *[]client.ScheduleRotationWorkingIntervalCreatePayloadV2 {
	payload := make([]client.ScheduleRotationWorkingIntervalCreatePayloadV2, len(windows))
	for i, window := range windows {
		payload[i] = client.ScheduleRotationWorkingIntervalCreatePayloadV2{
			Weekday:   client.ScheduleRotationWorkingIntervalCreatePayloadV2Weekday(window.Weekday.ValueString()),
			StartTime: window.StartTime.ValueString(),
			EndTime:   window.EndTime.ValueString(),
		}
	}
	return &payload
}

func rotationUpdatePayload(data IncidentScheduleRotationBetaModel, startsAt time.Time) client.ScheduleRotationUpdatePayloadV3 {
	payload := client.ScheduleRotationUpdatePayloadV3{
		Name:      data.Name.ValueString(),
		Users:     toUserReferences(data.Users),
		Handovers: toHandoversPayload(data.Handovers),
		// Sent even though the API keeps the current value when it's omitted, since
		// the config is what this rotation should look like.
		FirstIntervalStartsAt: &startsAt,
		ConcurrentShifts:      data.ConcurrentShifts.ValueInt64Pointer(),
		WorkingIntervals:      toWorkingIntervalsPayload(data.WorkingIntervals),
		Rank:                  data.Rank.ValueInt64Pointer(),
	}

	// A plan that dropped the attribute carries the mode forward from state, so this
	// is normally set either way — but a config building it from another resource's
	// output leaves it unknown, and the API keeps the current mode when it's omitted.
	if !data.SchedulingMode.IsNull() && !data.SchedulingMode.IsUnknown() {
		mode := client.ScheduleRotationUpdatePayloadV3SchedulingMode(data.SchedulingMode.ValueString())
		payload.SchedulingMode = &mode
	}

	return payload
}

// rotationInputsKnown reports whether the attributes describing the line-up have
// settled. One taken from a resource this apply hasn't created yet is unknown at
// plan time, which is a value IncidentScheduleRotationBetaModel has no way to hold — so this
// reads the plan's raw attributes rather than decoding it.
func rotationInputsKnown(plan tftypes.Value) bool {
	var attributes map[string]tftypes.Value
	if err := plan.As(&attributes); err != nil {
		return false
	}

	for _, name := range []string{
		"name", "users", "handovers", "first_interval_starts_at",
		"concurrent_shifts", "working_intervals", "rank", "scheduling_mode",
	} {
		if !attributes[name].IsFullyKnown() {
			return false
		}
	}

	return true
}

// rotationLineUpDiffers reports whether the plan changes the shape a rollout would
// introduce.
//
// Three attributes are deliberately left out. Name and rank belong to the rotation
// rather than to one of its shapes, so changing either applies across the lot and
// never disturbs who is on call. first_interval_starts_at is left out because
// phasing a change in re-anchors it anyway: routing a move through a rollout would
// throw the move away, where a plain edit simply makes it.
//
// scheduling_mode is in. It decides who gets allocated to which shift, so on a
// rotation with uneven handovers or working intervals — the rotations someone would
// set it on at all — changing it can move who is on call, which is the thing a
// rollout exists to hold back.
func rotationLineUpDiffers(plan, state IncidentScheduleRotationBetaModel) bool {
	return !plan.ConcurrentShifts.Equal(state.ConcurrentShifts) ||
		!plan.SchedulingMode.Equal(state.SchedulingMode) ||
		!reflect.DeepEqual(plan.Users, state.Users) ||
		!reflect.DeepEqual(plan.Handovers, state.Handovers) ||
		!reflect.DeepEqual(plan.WorkingIntervals, state.WorkingIntervals)
}

// anchorToState decides what first_interval_starts_at to record.
//
// Phasing a change in moves the anchor: the new line-up is cycled in from a future
// handover, and the anchor slides along the cadence to put the right person on call
// at it. That's the same handover time, so recording it would read as drift and
// phase the same change in again on the next apply. So when the value we're given
// lands on the same recurring slot as the config's, the config's is kept.
//
// Anything else is a genuinely different handover time — someone moved it, here or
// elsewhere — and has to surface.
func anchorToState(rotation client.ScheduleRotationV3, config timetypes.RFC3339, timezone string) timetypes.RFC3339 {
	server := timetypes.NewRFC3339TimeValue(rotation.FirstIntervalStartsAt)
	if config.IsNull() || config.IsUnknown() {
		return server
	}

	configured, diags := config.ValueRFC3339Time()
	if diags.HasError() {
		return server
	}

	// The same moment written another way. The attribute type reconciles "Z" with
	// "+00:00" itself, but reads two different offsets for one instant as unequal, so
	// a config written in a non-UTC offset would otherwise diff on every plan.
	if configured.Equal(rotation.FirstIntervalStartsAt) {
		return config
	}

	// No timezone to compare in, so there's no telling a slid anchor from a moved one.
	// Keep what was asked for rather than invent drift.
	if timezone == "" {
		return config
	}

	// Only well defined for a single cadence: with handovers that alternate there's no
	// one slot to be on.
	if len(rotation.Handovers) == 1 && sameHandoverSlot(rotation.FirstIntervalStartsAt,
		configured, string(rotation.Handovers[0].IntervalType), timezone) {
		return config
	}

	return server
}

// scheduleTimezone reads the timezone a rotation's handovers are anchored to. It's a
// property of the schedule, so it costs a second read.
//
// An empty return means it couldn't be resolved, which callers treat as "can't judge
// the anchor" rather than failing the whole operation over it.
func (r *IncidentScheduleRotationBetaResource) scheduleTimezone(ctx context.Context, scheduleID string) string {
	result, err := r.client.SchedulesV3ShowWithResponse(ctx, scheduleID)
	if err != nil || result.JSON200 == nil {
		return ""
	}

	return result.JSON200.Schedule.Timezone
}

// sameHandoverSlot reports whether two moments are the same recurring handover: the
// same weekday and time for a weekly cadence, the same time of day for a daily one,
// the same minute past the hour for an hourly one.
//
// The comparison is made in the schedule's timezone, so a handover at 09:00 local
// still matches itself either side of a daylight-saving change — the two are an hour
// apart in UTC.
//
// How many whole periods apart they are is deliberately not part of it. Phasing in a
// change to the cadence itself (weekly to fortnightly, say) anchors the new line-up
// off the old cadence's handover, which needn't sit on the new interval's grid at
// all; counting intervals would call that a change, and since the anchor moves again
// every time it's written, it would never settle.
func sameHandoverSlot(a, b time.Time, intervalType, timezone string) bool {
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return a.Equal(b)
	}

	first, second := a.In(location), b.In(location)
	sameClock := first.Hour() == second.Hour() &&
		first.Minute() == second.Minute() &&
		first.Second() == second.Second()

	switch intervalType {
	case "weekly":
		return first.Weekday() == second.Weekday() && sameClock
	case "daily":
		return sameClock
	case "hourly":
		return first.Minute() == second.Minute() && first.Second() == second.Second()
	default:
		return a.Equal(b)
	}
}

// incidentScheduleRotationBetaFromAPI projects an API rotation into Terraform state. config is
// whatever the plan or prior state held, which is needed to tell an unset rank apart
// from one the server happens to hold, and to keep the config's timestamp spelling.
func incidentScheduleRotationBetaFromAPI(rotation client.ScheduleRotationV3, config IncidentScheduleRotationBetaModel, timezone string) *IncidentScheduleRotationBetaModel {
	users := make([]types.String, len(rotation.Users))
	for i, user := range rotation.Users {
		users[i] = types.StringValue(user.Id)
	}

	handovers := make([]IncidentScheduleRotationBetaHandover, len(rotation.Handovers))
	for i, handover := range rotation.Handovers {
		handovers[i] = IncidentScheduleRotationBetaHandover{
			Interval:     types.Int64Value(handover.Interval),
			IntervalType: types.StringValue(string(handover.IntervalType)),
		}
	}

	// The API sends an empty list for a rotation with no restrictions, which config
	// expresses by omitting the attribute — so storing [] would diff on every plan.
	var windows []IncidentScheduleRotationBetaWorkingWindow
	if rotation.WorkingIntervals != nil && len(*rotation.WorkingIntervals) > 0 {
		windows = make([]IncidentScheduleRotationBetaWorkingWindow, len(*rotation.WorkingIntervals))
		for i, window := range *rotation.WorkingIntervals {
			windows[i] = IncidentScheduleRotationBetaWorkingWindow{
				Weekday:   types.StringValue(string(window.Weekday)),
				StartTime: types.StringValue(window.StartTime),
				EndTime:   types.StringValue(window.EndTime),
			}
		}
	}

	// Only track the mode when the config sets it, as with rank. A rotation that never
	// stated one comes back without it, so reading the response unconditionally would
	// leave the attribute unknown after an apply — which Terraform rejects outright.
	schedulingMode := types.StringNull()
	if !config.SchedulingMode.IsNull() && rotation.SchedulingMode != nil {
		schedulingMode = types.StringValue(string(*rotation.SchedulingMode))
	}

	// Only track rank when the config sets it — see the schema.
	rank := types.Int64Null()
	if !config.Rank.IsNull() && rotation.Rank != nil {
		rank = types.Int64Value(*rotation.Rank)
	}

	return &IncidentScheduleRotationBetaModel{
		ID:                    types.StringValue(rotation.Id),
		ScheduleID:            types.StringValue(rotation.ScheduleId),
		Name:                  types.StringValue(rotation.Name),
		Users:                 users,
		Handovers:             handovers,
		FirstIntervalStartsAt: anchorToState(rotation, config.FirstIntervalStartsAt, timezone),
		ConcurrentShifts:      types.Int64Value(rotation.ConcurrentShifts),
		WorkingIntervals:      windows,
		Rank:                  rank,
		SchedulingMode:        schedulingMode,
		// Ours to remember: it says how to introduce the next change, so the API has
		// nothing to return for it.
		Rollout:       config.Rollout,
		EffectiveFrom: timetypes.NewRFC3339TimePointerValue(rotation.EffectiveFrom),
	}
}
