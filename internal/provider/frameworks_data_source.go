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
	_ datasource.DataSource              = (*frameworksDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*frameworksDataSource)(nil)
)

type frameworksDataSource struct {
	client *client.Client
}

type frameworkItemModel struct {
	ID                   types.String  `tfsdk:"id"`
	DisplayName          types.String  `tfsdk:"display_name"`
	ShorthandName        types.String  `tfsdk:"shorthand_name"`
	Description          types.String  `tfsdk:"description"`
	NumControlsCompleted types.Float64 `tfsdk:"num_controls_completed"`
	NumControlsTotal     types.Float64 `tfsdk:"num_controls_total"`
	NumTestsPassing      types.Float64 `tfsdk:"num_tests_passing"`
	NumTestsTotal        types.Float64 `tfsdk:"num_tests_total"`
}

type frameworksDataModel struct {
	Frameworks []frameworkItemModel `tfsdk:"frameworks"`
}

func NewFrameworksDataSource() datasource.DataSource {
	return &frameworksDataSource{}
}

func (d *frameworksDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_frameworks"
}

func (d *frameworksDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "List the compliance frameworks configured in Vanta with their progress counters.",
		Attributes: map[string]schema.Attribute{
			"frameworks": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":                     schema.StringAttribute{Computed: true, Description: "Framework ID."},
						"display_name":           schema.StringAttribute{Computed: true, Description: "Framework display name."},
						"shorthand_name":         schema.StringAttribute{Computed: true, Description: "Short framework name."},
						"description":            schema.StringAttribute{Computed: true, Description: "Framework description."},
						"num_controls_completed": schema.Float64Attribute{Computed: true, Description: "Number of completed controls."},
						"num_controls_total":     schema.Float64Attribute{Computed: true, Description: "Total number of controls."},
						"num_tests_passing":      schema.Float64Attribute{Computed: true, Description: "Number of passing tests."},
						"num_tests_total":        schema.Float64Attribute{Computed: true, Description: "Total number of tests."},
					},
				},
			},
		},
	}
}

func (d *frameworksDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *frameworksDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	frameworks, err := d.client.ListFrameworks(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Failed to list frameworks", err.Error())
		return
	}

	out := make([]frameworkItemModel, 0, len(frameworks))
	for i := range frameworks {
		f := &frameworks[i]
		out = append(out, frameworkItemModel{
			ID:                   types.StringValue(f.ID),
			DisplayName:          types.StringValue(f.DisplayName),
			ShorthandName:        types.StringValue(f.ShorthandName),
			Description:          types.StringValue(f.Description),
			NumControlsCompleted: types.Float64Value(f.NumControlsCompleted),
			NumControlsTotal:     types.Float64Value(f.NumControlsTotal),
			NumTestsPassing:      types.Float64Value(f.NumTestsPassing),
			NumTestsTotal:        types.Float64Value(f.NumTestsTotal),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &frameworksDataModel{Frameworks: out})...)
}
