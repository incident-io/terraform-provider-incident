package provider

import (
	"context"
	"fmt"
	"sync"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/incident-io/terraform-provider-incident/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/internal/client"
	"github.com/incident-io/terraform-provider-incident/internal/provider/models"
)

var (
	_ datasource.DataSource              = &IncidentScheduleDataSource{}
	_ datasource.DataSourceWithConfigure = &IncidentScheduleDataSource{}
)

func NewIncidentScheduleDataSource() datasource.DataSource {
	return &IncidentScheduleDataSource{}
}

type IncidentScheduleDataSource struct {
	client                    *client.ClientWithResponses
	scheduleTypeID            string
	scheduleTypeIDOnce        sync.Once
	scheduleTypeIDLookupError error
}

type IncidentScheduleDataSourceModel struct {
	ID       types.String `tfsdk:"id"`
	Name     types.String `tfsdk:"name"`
	Timezone types.String `tfsdk:"timezone"`
	TeamIDs  types.Set    `tfsdk:"team_ids"`
}

func (d *IncidentScheduleDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*IncidentProviderData)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *IncidentProviderData, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	d.client = client.Client
}

func (d *IncidentScheduleDataSource) getScheduleTypeID(ctx context.Context) (string, error) {
	d.scheduleTypeIDOnce.Do(func() {
		typesResult, err := d.client.CatalogV3ListTypesWithResponse(ctx)
		if err == nil && typesResult.StatusCode() >= 400 {
			err = fmt.Errorf(string(typesResult.Body))
		}
		if err != nil {
			d.scheduleTypeIDLookupError = fmt.Errorf("unable to list catalog types, got error: %s", err)
			return
		}

		for _, catalogType := range typesResult.JSON200.CatalogTypes {
			if catalogType.Name == "Schedule" {
				d.scheduleTypeID = catalogType.Id
				return
			}
		}

		d.scheduleTypeIDLookupError = fmt.Errorf("schedule catalog type not found")
	})

	if d.scheduleTypeIDLookupError != nil {
		return "", d.scheduleTypeIDLookupError
	}

	return d.scheduleTypeID, nil
}

func (d *IncidentScheduleDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_schedule"
}

func (d *IncidentScheduleDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data IncidentScheduleDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var schedule *client.ScheduleV2
	if !data.ID.IsNull() {
		// Lookup by ID
		result, err := d.client.SchedulesV2ShowWithResponse(ctx, data.ID.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read schedule, got error: %s", err))
			return
		}
		schedule = &result.JSON200.Schedule
	} else if !data.Name.IsNull() {
		// Lookup by name using catalog API
		scheduleName := data.Name.ValueString()

		// Step 1: Get the cached Schedule catalog type ID
		scheduleTypeID, err := d.getScheduleTypeID(ctx)
		if err != nil {
			resp.Diagnostics.AddError("Client Error", err.Error())
			return
		}

		// Step 2: Find catalog entry by name
		entriesResult, err := d.client.CatalogV3ListEntriesWithResponse(ctx, &client.CatalogV3ListEntriesParams{
			CatalogTypeId: scheduleTypeID,
			Identifier:    &scheduleName,
			PageSize:      1,
		})
		if err == nil && entriesResult.StatusCode() >= 400 {
			err = fmt.Errorf(string(entriesResult.Body))
		}
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to list catalog entries, got error: %s", err))
			return
		}

		if len(entriesResult.JSON200.CatalogEntries) == 0 {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to find schedule with name: %s", scheduleName))
			return
		}

		catalogEntry := entriesResult.JSON200.CatalogEntries[0]
		if catalogEntry.ExternalId == nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Catalog entry for schedule '%s' has no external ID", scheduleName))
			return
		}

		// Step 3: Fetch schedule by ID using the external ID from catalog entry
		scheduleID := *catalogEntry.ExternalId
		result, err := d.client.SchedulesV2ShowWithResponse(ctx, scheduleID)
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read schedule with ID %s, got error: %s", scheduleID, err))
			return
		}
		schedule = &result.JSON200.Schedule
	} else {
		resp.Diagnostics.AddError("Client Error", "Either 'id' or 'name' must be provided")
		return
	}

	// Build the model using the same method as the resource
	scheduleResource := &IncidentScheduleResource{}
	emptyPlan := &models.IncidentScheduleResourceModelV2{
		TeamIDs: types.SetNull(types.StringType),
	}
	resourceModel := scheduleResource.buildModel(*schedule, emptyPlan)

	// Convert to data source model
	modelResp := &IncidentScheduleDataSourceModel{
		ID:       resourceModel.ID,
		Name:     resourceModel.Name,
		Timezone: resourceModel.Timezone,
		TeamIDs:  resourceModel.TeamIDs,
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &modelResp)...)
}

func (d *IncidentScheduleDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: fmt.Sprintf("%s\n\n%s", apischema.TagDocstring("Schedules V2"), "Use this data source to retrieve information about an existing schedule."),
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: apischema.Docstring("ScheduleV2", "id"),
			},
			"name": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: apischema.Docstring("ScheduleV2", "name"),
			},
			"timezone": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("ScheduleV2", "timezone"),
			},
			"team_ids": schema.SetAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: apischema.Docstring("ScheduleV2", "team_ids"),
			},
		},
	}
}
