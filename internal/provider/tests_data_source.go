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
	_ datasource.DataSource              = (*testsDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*testsDataSource)(nil)
)

type testsDataSource struct {
	client *client.Client
}

type testItemModel struct {
	ID              types.String `tfsdk:"id"`
	Name            types.String `tfsdk:"name"`
	Status          types.String `tfsdk:"status"`
	Category        types.String `tfsdk:"category"`
	LastTestRunDate types.String `tfsdk:"last_test_run_date"`
}

type testsDataModel struct {
	StatusFilter      types.String    `tfsdk:"status_filter"`
	FrameworkFilter   types.String    `tfsdk:"framework_filter"`
	IntegrationFilter types.String    `tfsdk:"integration_filter"`
	Tests             []testItemModel `tfsdk:"tests"`
}

func NewTestsDataSource() datasource.DataSource {
	return &testsDataSource{}
}

func (d *testsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_tests"
}

func (d *testsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "List Vanta tests, optionally filtered by status, framework, or integration.",
		Attributes: map[string]schema.Attribute{
			"status_filter": schema.StringAttribute{
				Optional:    true,
				Description: "Filter by test status (e.g. `OK`, `NEEDS_ATTENTION`, `DEACTIVATED`).",
			},
			"framework_filter": schema.StringAttribute{
				Optional:    true,
				Description: "Filter by framework.",
			},
			"integration_filter": schema.StringAttribute{
				Optional:    true,
				Description: "Filter by integration.",
			},
			"tests": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":                 schema.StringAttribute{Computed: true, Description: "Test ID."},
						"name":               schema.StringAttribute{Computed: true, Description: "Test name."},
						"status":             schema.StringAttribute{Computed: true, Description: "Test status."},
						"category":           schema.StringAttribute{Computed: true, Description: "Test category."},
						"last_test_run_date": schema.StringAttribute{Computed: true, Description: "Timestamp of the last test run."},
					},
				},
			},
		},
	}
}

func (d *testsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *testsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config testsDataModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tests, err := d.client.ListTests(ctx, client.ListTestsFilter{
		StatusFilter:      config.StatusFilter.ValueString(),
		FrameworkFilter:   config.FrameworkFilter.ValueString(),
		IntegrationFilter: config.IntegrationFilter.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Failed to list tests", err.Error())
		return
	}

	config.Tests = make([]testItemModel, 0, len(tests))
	for i := range tests {
		t := &tests[i]
		config.Tests = append(config.Tests, testItemModel{
			ID:              types.StringValue(t.ID),
			Name:            types.StringValue(t.Name),
			Status:          types.StringValue(t.Status),
			Category:        types.StringValue(t.Category),
			LastTestRunDate: stringFromEmpty(t.LastTestRunDate),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
