package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/v6/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/v6/internal/client"
)

var (
	_ resource.Resource                = &IncidentAlertAttributeResource{}
	_ resource.ResourceWithImportState = &IncidentAlertAttributeResource{}
)

type IncidentAlertAttributeResource struct {
	resourceConfigurer
}

type IncidentAlertAttributeResourceModel struct {
	ID       types.String `tfsdk:"id"`
	Name     types.String `tfsdk:"name"`
	Type     types.String `tfsdk:"type"`
	Array    types.Bool   `tfsdk:"array"`
	Required types.Bool   `tfsdk:"required"`
	Emoji    types.String `tfsdk:"emoji"`
}

func NewIncidentAlertAttributeResource() resource.Resource {
	return &IncidentAlertAttributeResource{}
}

func (r *IncidentAlertAttributeResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_alert_attribute"
}

func (r *IncidentAlertAttributeResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: fmt.Sprintf("%s\n\n%s", apischema.TagDocstring("Alert Attributes V2"), `## An attribute is account-wide; what fills it in is per source

An alert attribute is the column, not the rule that populates it. It belongs to your whole
incident.io organization: its `+"`name`"+` has to be unique across the account, and every alert
source draws on the same set of attributes. That is deliberate — one attribute means the same
thing whichever source fired the alert, so an alert route spanning several sources can match
on it.

What varies per source is the *binding*: the rule parsing a value for this attribute out of an
incoming event. A source binds a given attribute at most once, and two sources can fill the
same attribute from completely different parts of their payloads. Write bindings either as
`+"`template.attributes`"+` on `+"`incident_alert_source`"+`, which declares a source and everything it
populates together, or as one `+"`incident_alert_source_attribute_beta`"+` resource per binding
against an `+"`incident_alert_source_beta`"+` source.

So a `+"`GCP service`"+` attribute is declared once, then bound separately by each source that sets it.

## Declaring one attribute from several workspaces

If your Terraform is split across workspaces — one per environment, say — only one of them
should declare an attribute as a resource. The others should read it with the
`+"`incident_alert_attribute`"+` data source, which looks an attribute up by name:

    data "incident_alert_attribute" "gcp_service" {
      name = "GCP service"
    }

Declaring the same `+"`name`"+` in two workspaces plans cleanly in both, then fails when the second
applies: the attribute it means to create already exists. To bring an attribute that already
exists under a workspace's management instead, `+"`import`"+` it.

Per-environment differences belong on the binding rather than the attribute, so both
environments share one attribute and each parses it from its own source.

## Reserved names

`+"`Title`"+`, `+"`Description`"+` and `+"`Priority`"+` collide with properties every alert already has, and are
rejected regardless of casing. `+"`Priority`"+` does exist as an attribute you can bind and filter on,
but it is read-only through the API: read it with the data source rather than declaring it.`),
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("AlertAttributeV2", "id"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("AlertAttributeV2", "name"),
				Required:            true,
			},
			"type": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("AlertAttributeV2", "type"),
				Required:            true,
			},
			"array": schema.BoolAttribute{
				MarkdownDescription: apischema.Docstring("AlertAttributeV2", "array"),
				Required:            true,
			},
			"required": schema.BoolAttribute{
				MarkdownDescription: apischema.Docstring("AlertAttributeV2", "required"),
				Optional:            true,
				Computed:            true,
			},
			"emoji": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("AlertAttributeV2", "emoji"),
				Optional:            true,
			},
		},
	}
}

func (r *IncidentAlertAttributeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data *IncidentAlertAttributeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	requestBody := client.AlertAttributesCreatePayloadV2{
		Name:     data.Name.ValueString(),
		Type:     data.Type.ValueString(),
		Array:    data.Array.ValueBool(),
		Required: data.Required.ValueBoolPointer(),
		Emoji:    data.Emoji.ValueStringPointer(),
	}

	result, err := lockForAlertConfig(ctx, func(ctx context.Context) (*client.AlertAttributesV2CreateResponse, error) {
		result, err := r.client.AlertAttributesV2CreateWithResponse(ctx, requestBody)
		if err != nil {
			return result, err
		}
		return result, nil
	})
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create alert attribute, got error: %s", err))
		return
	}

	tflog.Trace(ctx, fmt.Sprintf("created an alert attribute resource with id=%s", result.JSON201.AlertAttribute.Id))
	data = r.buildModel(result.JSON201.AlertAttribute, data.Required)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *IncidentAlertAttributeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data *IncidentAlertAttributeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.AlertAttributesV2ShowWithResponse(ctx, data.ID.ValueString())
	if err != nil {
		// Check if error message contains any indication of a 404 not found
		httpErr := client.HTTPError{}
		if errors.As(err, &httpErr) && httpErr.StatusCode == 404 {
			tflog.Warn(ctx, fmt.Sprintf("Alert attribute with ID %s not found: removing from state.", data.ID.ValueString()))
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read alert attribute, got error: %s", err))
		return
	}

	data = r.buildModel(result.JSON200.AlertAttribute, data.Required)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *IncidentAlertAttributeResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data *IncidentAlertAttributeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	requestBody := client.AlertAttributesV2UpdateJSONRequestBody{
		Name:  data.Name.ValueString(),
		Type:  data.Type.ValueString(),
		Array: data.Array.ValueBool(),
	}
	if !data.Required.IsNull() {
		requestBody.Required = data.Required.ValueBoolPointer()
	}
	if data.Emoji.IsNull() {
		requestBody.Emoji = lo.ToPtr("")
	} else {
		requestBody.Emoji = data.Emoji.ValueStringPointer()
	}

	result, err := lockForAlertConfig(ctx, func(ctx context.Context) (*client.AlertAttributesV2UpdateResponse, error) {
		result, err := r.client.AlertAttributesV2UpdateWithResponse(ctx, data.ID.ValueString(), requestBody)
		if err != nil {
			return result, err
		}
		return result, nil
	})
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update alert attribute, got error: %s", err))
		return
	}

	data = r.buildModel(result.JSON200.AlertAttribute, data.Required)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *IncidentAlertAttributeResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data *IncidentAlertAttributeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := lockForAlertConfig(ctx, func(ctx context.Context) (*client.AlertAttributesV2DestroyResponse, error) {
		return r.client.AlertAttributesV2DestroyWithResponse(ctx, data.ID.ValueString())
	})
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete alert attribute, got error: %s", err))
		return
	}
}

func (r *IncidentAlertAttributeResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)

	// After import, we need to read the full resource and set all attributes
	// including the required field based on API response
	if resp.Diagnostics.HasError() {
		return
	}

	// Get the resource data from API
	result, err := r.client.AlertAttributesV2ShowWithResponse(ctx, req.ID)
	if err != nil {
		// Check if error message contains any indication of a 404 not found
		httpErr := client.HTTPError{}
		if errors.As(err, &httpErr) && httpErr.StatusCode == 404 {
			tflog.Warn(ctx, fmt.Sprintf("Alert attribute with ID %s not found: removing from state.", req.ID))
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read alert attribute, got error: %s", err))
		return
	}

	// For import, always set required based on API response
	data := r.buildModel(result.JSON200.AlertAttribute, types.BoolValue(result.JSON200.AlertAttribute.Required))
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *IncidentAlertAttributeResource) buildModel(alertAttribute client.AlertAttributeV2, configuredRequired types.Bool) *IncidentAlertAttributeResourceModel {
	model := &IncidentAlertAttributeResourceModel{
		ID:    types.StringValue(alertAttribute.Id),
		Name:  types.StringValue(alertAttribute.Name),
		Type:  types.StringValue(alertAttribute.Type),
		Array: types.BoolValue(alertAttribute.Array),
	}

	// Only set Required if it was explicitly configured, otherwise keep it null
	if configuredRequired.IsNull() {
		model.Required = types.BoolNull()
	} else {
		model.Required = types.BoolValue(alertAttribute.Required)
	}

	// Set emoji from API response, will be null if not set or empty
	if alertAttribute.Emoji != nil && *alertAttribute.Emoji != "" {
		model.Emoji = types.StringValue(*alertAttribute.Emoji)
	} else {
		model.Emoji = types.StringNull()
	}

	return model
}
