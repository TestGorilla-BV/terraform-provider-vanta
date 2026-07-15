package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/TestGorilla-BV/terraform-provider-vanta/internal/client"
)

var (
	_ datasource.DataSource              = (*peopleDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*peopleDataSource)(nil)
)

type peopleDataSource struct {
	client *client.Client
}

type personItemModel struct {
	ID               types.String `tfsdk:"id"`
	Email            types.String `tfsdk:"email"`
	DisplayName      types.String `tfsdk:"display_name"`
	EmploymentStatus types.String `tfsdk:"employment_status"`
	JobTitle         types.String `tfsdk:"job_title"`
}

type peopleDataModel struct {
	EmploymentStatus types.String      `tfsdk:"employment_status"`
	People           []personItemModel `tfsdk:"people"`
}

func NewPeopleDataSource() datasource.DataSource {
	return &peopleDataSource{}
}

func (d *peopleDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_people"
}

func (d *peopleDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "List people, optionally filtered by employment status.",
		Attributes: map[string]schema.Attribute{
			"employment_status": schema.StringAttribute{
				Optional:    true,
				Description: "Filter by employment status (e.g. `CURRENT`, `FORMER`, `ON_LEAVE`).",
			},
			"people": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":                schema.StringAttribute{Computed: true, Description: "Person ID."},
						"email":             schema.StringAttribute{Computed: true, Description: "Email address."},
						"display_name":      schema.StringAttribute{Computed: true, Description: "Display name."},
						"employment_status": schema.StringAttribute{Computed: true, Description: "Employment status."},
						"job_title":         schema.StringAttribute{Computed: true, Description: "Job title."},
					},
				},
			},
		},
	}
}

func (d *peopleDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("got %T", req.ProviderData))
		return
	}
	d.client = c
}

func (d *peopleDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config peopleDataModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	people, err := d.client.ListPeople(ctx, client.ListPeopleFilter{
		EmploymentStatus: config.EmploymentStatus.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Failed to list people", err.Error())
		return
	}

	config.People = make([]personItemModel, 0, len(people))
	for i := range people {
		p := &people[i]
		config.People = append(config.People, personItemModel{
			ID:               types.StringValue(p.ID),
			Email:            types.StringValue(p.EmailAddress),
			DisplayName:      types.StringValue(p.Name.Display),
			EmploymentStatus: types.StringValue(p.Employment.Status),
			JobTitle:         stringFromPtr(p.Employment.JobTitle),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
