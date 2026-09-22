package provider

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timetypes/timetypes"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/incident-io/terraform-provider-incident/v7/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/v7/internal/client"
	"github.com/incident-io/terraform-provider-incident/v7/internal/provider/models"
	"github.com/incident-io/terraform-provider-incident/v7/internal/provider/timestamptypes"
)

var (
	_ resource.Resource                   = &IncidentPayConfigResource{}
	_ resource.ResourceWithConfigure      = &IncidentPayConfigResource{}
	_ resource.ResourceWithImportState    = &IncidentPayConfigResource{}
	_ resource.ResourceWithValidateConfig = &IncidentPayConfigResource{}
)

// payConfigCurrencyPattern is the shape of an ISO 4217 currency code. The API stores
// whatever it's given and reads it back the same way, so this is the one place a typo
// is caught before it reaches a pay report.
var payConfigCurrencyPattern = regexp.MustCompile(`^[A-Za-z]{3}$`)

var (
	payConfigRateTimeUnits = enumValues("PayConfigV2", "rate_time_unit")
	payConfigWeekdays      = enumValues("PayConfigWeeklyRuleV2", "weekdays")
)

type IncidentPayConfigResource struct {
	resourceConfigurer
}

func NewIncidentPayConfigResource() resource.Resource {
	return &IncidentPayConfigResource{}
}

func (r *IncidentPayConfigResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pay_config"
}

func (r *IncidentPayConfigResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: fmt.Sprintf("%s\n\n%s", apischema.TagDocstring("Pay Configs V2"),
			`A pay config sets what someone is paid for being on call: a base rate, plus rules that
override it at particular times. An on-call pay report prices each person's shifts against one.

`+"`weekly_rules`"+` is ordered. A shift is priced by the first rule that covers it, so put the
rule that should win first. `+"`one_off_rules`"+` take precedence over every weekly rule and may
not overlap each other, so their order has no effect on pricing. Any time no rule covers is paid
at `+"`base_rate_cents`"+`.

Terraform creates the config as a draft, which becomes visible to everyone in the organisation
once a report that prices against it is published. Editing a config after that changes the
explanation of pay someone has already been sent, so it additionally needs the
`+"`schedule_pay_configs.update_published`"+` scope on the API key, which the `+"`pay_configs_editor`"+`
role does not carry. The provider only sends what changed, so a plan that touches nothing but
the rules never asks to update the config itself.`),
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("PayConfigV2", "id"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: apischema.Docstring("PayConfigV2", "name"),
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"timezone": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: apischema.Docstring("PayConfigV2", "timezone") + ", such as `Europe/London`.",
			},
			"currency": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: apischema.Docstring("PayConfigV2", "currency") + ", such as `GBP`.",
				Validators: []validator.String{
					stringvalidator.RegexMatches(payConfigCurrencyPattern, "must be a three-letter ISO 4217 currency code, such as GBP or USD"),
				},
			},
			"base_rate_cents": schema.Int64Attribute{
				Required:            true,
				MarkdownDescription: apischema.Docstring("PayConfigV2", "base_rate_cents"),
				Validators: []validator.Int64{
					int64validator.AtLeast(0),
				},
			},
			"rate_time_unit": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: EnumValuesDescription("PayConfigV2", "rate_time_unit"),
				Validators: []validator.String{
					stringvalidator.OneOf(payConfigRateTimeUnits...),
				},
			},
			"weekly_rules": schema.ListNestedAttribute{
				Optional: true,
				Computed: true,
				Default: listdefault.StaticValue(types.ListValueMust(
					types.ObjectType{AttrTypes: models.PayConfigWeeklyRuleModel{}.AttrTypes()}, []attr.Value{},
				)),
				MarkdownDescription: apischema.Docstring("PayConfigV2", "weekly_rules") +
					". The first rule that covers a shift prices it, so order matters. Changing the list " +
					"changes the rules in place, position by position: a rule that moves keeps its position's " +
					"ID rather than its own.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: apischema.Docstring("PayConfigWeeklyRuleV2", "id"),
							PlanModifiers: []planmodifier.String{
								useStateForUnknownIfSet{},
							},
						},
						"weekdays": schema.SetAttribute{
							Required:            true,
							ElementType:         types.StringType,
							MarkdownDescription: DescribeEnumValues(apischema.Docstring("PayConfigWeeklyRuleV2", "weekdays"), "PayConfigWeeklyRuleV2", "weekdays"),
							Validators: []validator.Set{
								setvalidator.SizeAtLeast(1),
								setvalidator.ValueStringsAre(stringvalidator.OneOf(payConfigWeekdays...)),
							},
						},
						"start_time": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: apischema.Docstring("PayConfigWeeklyRuleV2", "start_time") + ", as `HH:MM`.",
						},
						"end_time": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: apischema.Docstring("PayConfigWeeklyRuleV2", "end_time") + " Written as `HH:MM`.",
						},
						"rate_cents": schema.Int64Attribute{
							Required:            true,
							MarkdownDescription: apischema.Docstring("PayConfigWeeklyRuleV2", "rate_cents"),
							Validators: []validator.Int64{
								int64validator.AtLeast(0),
							},
						},
					},
				},
			},
			"one_off_rules": schema.ListNestedAttribute{
				Optional: true,
				Computed: true,
				Default: listdefault.StaticValue(types.ListValueMust(
					types.ObjectType{AttrTypes: models.PayConfigOneOffRuleModel{}.AttrTypes()}, []attr.Value{},
				)),
				MarkdownDescription: apischema.Docstring("PayConfigV2", "one_off_rules") +
					". These take precedence over the weekly rules and may not overlap each other. " +
					"Changing the list changes the rules in place, position by position.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: apischema.Docstring("PayConfigOneOffRuleV2", "id"),
							PlanModifiers: []planmodifier.String{
								useStateForUnknownIfSet{},
							},
						},
						"name": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: apischema.Docstring("PayConfigOneOffRuleV2", "name"),
							Validators: []validator.String{
								stringvalidator.LengthAtLeast(1),
							},
						},
						"start_at": schema.StringAttribute{
							Required:            true,
							CustomType:          timestamptypes.InstantType{},
							MarkdownDescription: apischema.Docstring("PayConfigOneOffRuleV2", "start_at") + ", as an RFC 3339 timestamp. Any offset works: the API reports the same moment in UTC, and that is not a change.",
						},
						"end_at": schema.StringAttribute{
							Required:            true,
							CustomType:          timestamptypes.InstantType{},
							MarkdownDescription: apischema.Docstring("PayConfigOneOffRuleV2", "end_at") + ", as an RFC 3339 timestamp.",
						},
						"rate_cents": schema.Int64Attribute{
							Required:            true,
							MarkdownDescription: apischema.Docstring("PayConfigOneOffRuleV2", "rate_cents"),
							Validators: []validator.Int64{
								int64validator.AtLeast(0),
							},
						},
					},
				},
			},
			"published_at": schema.StringAttribute{
				Computed:            true,
				CustomType:          timetypes.RFC3339Type{},
				MarkdownDescription: apischema.Docstring("PayConfigV2", "published_at") + " Null until then.",
			},
			"created_at": schema.StringAttribute{
				Computed:   true,
				CustomType: timetypes.RFC3339Type{},
				// The API schema has no description for either timestamp, so these are the
				// provider's own words rather than apischema.Docstring.
				MarkdownDescription: "When this pay config was created.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"updated_at": schema.StringAttribute{
				Computed:            true,
				CustomType:          timetypes.RFC3339Type{},
				MarkdownDescription: "When this pay config, or one of its rules, was last changed.",
			},
		},
	}
}

// ValidateConfig checks what the schema can't, so a bad config fails at plan time rather
// than part-way through an apply. Each check mirrors one the API makes, or catches a value
// the API would store as written and only trip over when a report is generated. A value
// that isn't known until apply is left alone.
func (r *IncidentPayConfigResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	r.validateTimezone(ctx, req, resp)
	r.validateWeeklyRules(ctx, req, resp)
	r.validateOneOffRules(ctx, req, resp)
}

func (r *IncidentPayConfigResource) validateTimezone(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	timezonePath := path.Root("timezone")

	var timezone types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, timezonePath, &timezone)...)
	if resp.Diagnostics.HasError() || timezone.IsNull() || timezone.IsUnknown() {
		return
	}

	// LoadLocation resolves "" to UTC and "Local" to the machine's zone, neither of which
	// is a name a pay report can interpret the rules in.
	value := timezone.ValueString()
	if _, err := time.LoadLocation(value); err != nil || value == "" || value == "Local" {
		resp.Diagnostics.AddAttributeError(
			timezonePath,
			"Invalid timezone",
			fmt.Sprintf("%q isn't an IANA timezone name. Use a name like Europe/London or America/Los_Angeles.", value),
		)
	}
}

// validateWeeklyRules checks each rule's times are a time of day. The API accepts any two
// digits either side of a colon, so 25:99 would be stored and then never match a shift.
func (r *IncidentPayConfigResource) validateWeeklyRules(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	for _, rule := range knownListObjects(ctx, req, resp, path.Root("weekly_rules")) {
		for _, name := range []string{"start_time", "end_time"} {
			value, ok := knownString(rule.attributes[name])
			if !ok {
				continue
			}

			if _, ok := clockTimeMinutes(value); !ok {
				resp.Diagnostics.AddAttributeError(
					rule.path.AtName(name),
					fmt.Sprintf("Invalid %s", name),
					fmt.Sprintf("%q isn't a time of day. Use 24-hour HH:MM, like 09:00.", value),
				)
			}
		}
	}
}

// validateOneOffRules checks each rule starts before it ends, and that no two rules
// overlap. Both are checks the API makes when the config is written, so failing them here
// saves an apply that would fail part-way through: a config's rules are reconciled one at
// a time, and an overlap is only rejected when the second of the pair is written.
func (r *IncidentPayConfigResource) validateOneOffRules(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	type window struct {
		path       path.Path
		name       string
		start, end time.Time
	}

	var windows []window
	for _, rule := range knownListObjects(ctx, req, resp, path.Root("one_off_rules")) {
		start, startKnown := knownInstant(rule.attributes["start_at"])
		end, endKnown := knownInstant(rule.attributes["end_at"])
		if !startKnown || !endKnown {
			continue
		}

		if start.After(end) {
			resp.Diagnostics.AddAttributeError(
				rule.path.AtName("start_at"),
				"Rule ends before it starts",
				"start_at must be before end_at.",
			)
			continue
		}

		name, _ := knownString(rule.attributes["name"])
		windows = append(windows, window{path: rule.path, name: name, start: start, end: end})
	}

	// The same test the API applies: a rule overlaps another when it strictly contains
	// either end of it. Two rules over exactly the same window pass, as they do there.
	for i, this := range windows {
		for j, that := range windows {
			if i == j {
				continue
			}

			containsStart := this.start.Before(that.start) && this.end.After(that.start)
			containsEnd := this.start.Before(that.end) && this.end.After(that.end)
			if containsStart || containsEnd {
				resp.Diagnostics.AddAttributeError(
					this.path,
					"One-off rules overlap",
					fmt.Sprintf("Rule %q overlaps rule %q. One-off rules may not overlap: at most one applies to any moment.", this.name, that.name),
				)
				break
			}
		}
	}
}

// listObject is one known element of a list of objects, with the path to report a problem
// against.
type listObject struct {
	path       path.Path
	attributes map[string]attr.Value
}

// knownListObjects reads the known elements of a list attribute. A list, or an element,
// that isn't known until apply is skipped: there's nothing to judge yet.
func knownListObjects(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse, listPath path.Path) []listObject {
	var list types.List
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, listPath, &list)...)
	if resp.Diagnostics.HasError() || list.IsNull() || list.IsUnknown() {
		return nil
	}

	var objects []listObject
	for idx, element := range list.Elements() {
		object, ok := element.(types.Object)
		if !ok || object.IsNull() || object.IsUnknown() {
			continue
		}

		objects = append(objects, listObject{
			path:       listPath.AtListIndex(idx),
			attributes: object.Attributes(),
		})
	}

	return objects
}

// knownInstant is knownString for a timestamp attribute. The custom type has already
// rejected a value that doesn't parse, so one that fails here is treated as unknown.
func knownInstant(value attr.Value) (time.Time, bool) {
	instant, ok := value.(timestamptypes.Instant)
	if !ok {
		return time.Time{}, false
	}

	return instant.ValueTime()
}

func (r *IncidentPayConfigResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data models.PayConfigModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.PayConfigsV2CreateWithResponse(ctx, data.ToCreatePayload())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create pay config '%s', got error: %s", data.Name.ValueString(), err))
		return
	}
	if result.JSON201 == nil {
		resp.Diagnostics.AddError(
			"Client Error",
			fmt.Sprintf("Unable to create pay config: unexpected response from API (status %s)", result.Status()),
		)
		return
	}

	tflog.Trace(ctx, fmt.Sprintf("created a pay config with id=%s", result.JSON201.PayConfig.Id))

	state := models.PayConfigModel{}.FromAPI(result.JSON201.PayConfig)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *IncidentPayConfigResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data models.PayConfigModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	config, err := r.show(ctx, data.ID.ValueString())
	if err != nil {
		httpErr := client.HTTPError{}
		if errors.As(err, &httpErr) && httpErr.StatusCode == 404 {
			tflog.Warn(ctx, fmt.Sprintf("Pay config with ID %s not found: removing from state.", data.ID.ValueString()))
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read pay config, got error: %s", err))
		return
	}

	state := models.PayConfigModel{}.FromAPI(*config)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update reconciles the config in three parts, each through its own endpoint: the
// config's attributes, then the weekly rules, then the one-off rules. Only what changed
// is sent, so a config a published report priced against can have a rule added without
// the scope that editing the config itself would need. State is read back whole at the
// end, so a failure part-way through leaves state describing what was actually applied
// and the next plan picks up the rest.
func (r *IncidentPayConfigResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state models.PayConfigModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := state.ID.ValueString()

	if !plan.AttributesEqual(state) {
		result, err := r.client.PayConfigsV2UpdateWithResponse(ctx, id, plan.ToUpdatePayload())
		if err == nil && result.JSON200 == nil {
			err = fmt.Errorf("unexpected response from API (status %s)", result.Status())
		}
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update pay config, got error: %s", err))
			return
		}
	}

	r.reconcileWeeklyRules(ctx, id, plan.WeeklyRules, state.WeeklyRules, &resp.Diagnostics)
	r.reconcileOneOffRules(ctx, id, plan.OneOffRules, state.OneOffRules, &resp.Diagnostics)

	// Whatever happened above, the config as the API now has it is the truth: a partial
	// failure needs state to say how far the apply got, and a success needs the IDs of any
	// rules that were added.
	config, err := r.show(ctx, id)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read pay config after updating it, got error: %s", err))
		return
	}

	newState := models.PayConfigModel{}.FromAPI(*config)
	resp.Diagnostics.Append(resp.State.Set(ctx, &newState)...)
}

// reconcileWeeklyRules brings the config's weekly rules to the planned list, position by
// position. The order of the list is the order rules are evaluated in, and the API offers
// no way to reorder them: a rule can be changed in place, appended, or removed. Matching
// by position uses exactly those three moves. A rule at a position both lists have is
// updated in place if it differs; positions only the plan has are appended, in order;
// positions only the state has are removed.
//
// That is also what the plan promised. Each position's ID is carried over from state by
// UseStateForUnknown, so the rule at a position must keep that ID, whatever it now says.
func (r *IncidentPayConfigResource) reconcileWeeklyRules(
	ctx context.Context, configID string,
	planned, current []models.PayConfigWeeklyRuleModel,
	diags *diag.Diagnostics,
) {
	shared := min(len(planned), len(current))

	for idx := shared; idx < len(current); idx++ {
		ruleID := current[idx].ID.ValueString()
		if _, err := r.client.PayConfigsV2DestroyWeeklyRuleWithResponse(ctx, configID, ruleID); err != nil {
			diags.AddError("Client Error", fmt.Sprintf("Unable to remove weekly rule %s (position %d), got error: %s", ruleID, idx, err))
			return
		}
	}

	for idx := range shared {
		if planned[idx].Equivalent(current[idx]) {
			continue
		}

		ruleID := current[idx].ID.ValueString()
		result, err := r.client.PayConfigsV2UpdateWeeklyRuleWithResponse(ctx, configID, ruleID, planned[idx].ToUpdatePayload())
		if err == nil && result.JSON200 == nil {
			err = fmt.Errorf("unexpected response from API (status %s)", result.Status())
		}
		if err != nil {
			diags.AddError("Client Error", fmt.Sprintf("Unable to update weekly rule %s (position %d), got error: %s", ruleID, idx, err))
			return
		}
	}

	for idx := shared; idx < len(planned); idx++ {
		result, err := r.client.PayConfigsV2CreateWeeklyRuleWithResponse(ctx, configID, planned[idx].ToCreatePayload())
		if err == nil && result.JSON201 == nil {
			err = fmt.Errorf("unexpected response from API (status %s)", result.Status())
		}
		if err != nil {
			diags.AddError("Client Error", fmt.Sprintf("Unable to add weekly rule at position %d, got error: %s", idx, err))
			return
		}
	}
}

// reconcileOneOffRules is reconcileWeeklyRules for the one-off rules, and matches by
// position for the same reason: the plan has already promised each position its ID.
//
// Removals go first and additions last, so a config that swaps one holiday for another
// at the same dates never holds both at once. The API rejects a rule that overlaps
// another, and that check runs on every write.
func (r *IncidentPayConfigResource) reconcileOneOffRules(
	ctx context.Context, configID string,
	planned, current []models.PayConfigOneOffRuleModel,
	diags *diag.Diagnostics,
) {
	shared := min(len(planned), len(current))

	for idx := shared; idx < len(current); idx++ {
		ruleID := current[idx].ID.ValueString()
		if _, err := r.client.PayConfigsV2DestroyOneOffRuleWithResponse(ctx, configID, ruleID); err != nil {
			diags.AddError("Client Error", fmt.Sprintf("Unable to remove one-off rule %s (position %d), got error: %s", ruleID, idx, err))
			return
		}
	}

	for idx := range shared {
		if planned[idx].Equivalent(current[idx]) {
			continue
		}

		ruleID := current[idx].ID.ValueString()
		result, err := r.client.PayConfigsV2UpdateOneOffRuleWithResponse(ctx, configID, ruleID, planned[idx].ToUpdatePayload())
		if err == nil && result.JSON200 == nil {
			err = fmt.Errorf("unexpected response from API (status %s)", result.Status())
		}
		if err != nil {
			diags.AddError("Client Error", fmt.Sprintf("Unable to update one-off rule %s (position %d), got error: %s", ruleID, idx, err))
			return
		}
	}

	for idx := shared; idx < len(planned); idx++ {
		result, err := r.client.PayConfigsV2CreateOneOffRuleWithResponse(ctx, configID, planned[idx].ToCreatePayload())
		if err == nil && result.JSON201 == nil {
			err = fmt.Errorf("unexpected response from API (status %s)", result.Status())
		}
		if err != nil {
			diags.AddError("Client Error", fmt.Sprintf("Unable to add one-off rule at position %d, got error: %s", idx, err))
			return
		}
	}
}

func (r *IncidentPayConfigResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data models.PayConfigModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.client.PayConfigsV2DestroyWithResponse(ctx, data.ID.ValueString())
	if err != nil {
		// Already gone is the outcome a delete wants.
		httpErr := client.HTTPError{}
		if errors.As(err, &httpErr) && httpErr.StatusCode == 404 {
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete pay config, got error: %s", err))
		return
	}
}

func (r *IncidentPayConfigResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	config, err := r.show(ctx, strings.TrimSpace(req.ID))
	if err != nil {
		httpErr := client.HTTPError{}
		if errors.As(err, &httpErr) && httpErr.StatusCode == 404 {
			resp.Diagnostics.AddError(
				"Pay Config Not Found",
				fmt.Sprintf("No pay config with ID %q exists, or it is a draft belonging to somebody else.", req.ID),
			)
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read pay config, got error: %s", err))
		return
	}

	data := models.PayConfigModel{}.FromAPI(*config)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// show fetches a config, returning the client's HTTPError untouched so callers can tell
// a missing config from a failed request.
func (r *IncidentPayConfigResource) show(ctx context.Context, id string) (*client.PayConfigV2, error) {
	result, err := r.client.PayConfigsV2ShowWithResponse(ctx, id)
	if err != nil {
		return nil, err
	}
	if result.JSON200 == nil {
		return nil, fmt.Errorf("unexpected response from API (status %s)", result.Status())
	}

	return &result.JSON200.PayConfig, nil
}
