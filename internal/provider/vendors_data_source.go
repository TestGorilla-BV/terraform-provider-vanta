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
	_ datasource.DataSource              = (*vendorsDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*vendorsDataSource)(nil)
)

type vendorsDataSource struct {
	client *client.Client
}

type vendorItemModel struct {
	ID                types.String `tfsdk:"id"`
	Name              types.String `tfsdk:"name"`
	WebsiteURL        types.String `tfsdk:"website_url"`
	Status            types.String `tfsdk:"status"`
	InherentRiskLevel types.String `tfsdk:"inherent_risk_level"`
	ResidualRiskLevel types.String `tfsdk:"residual_risk_level"`
}

type vendorsDataModel struct {
	Name             types.String      `tfsdk:"name"`
	StatusMatchesAny types.List        `tfsdk:"status_matches_any"`
	Vendors          []vendorItemModel `tfsdk:"vendors"`
}

func NewVendorsDataSource() datasource.DataSource {
	return &vendorsDataSource{}
}

func (d *vendorsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vendors"
}

func (d *vendorsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "List vendors, optionally filtered by name and status.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Optional:    true,
				Description: "Filter vendors by name (case-insensitive partial match).",
			},
			"status_matches_any": schema.ListAttribute{
				Optional:    true,
				ElementType: types.StringType,
				Description: "Filter to vendors whose status is any of these values.",
			},
			"vendors": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":                  schema.StringAttribute{Computed: true, Description: "Vendor ID."},
						"name":                schema.StringAttribute{Computed: true, Description: "Vendor name."},
						"website_url":         schema.StringAttribute{Computed: true, Description: "Vendor website URL."},
						"status":              schema.StringAttribute{Computed: true, Description: "Vendor status."},
						"inherent_risk_level": schema.StringAttribute{Computed: true, Description: "Inherent risk level."},
						"residual_risk_level": schema.StringAttribute{Computed: true, Description: "Residual risk level."},
					},
				},
			},
		},
	}
}

func (d *vendorsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *vendorsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config vendorsDataModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	filter := client.ListVendorsFilter{Name: config.Name.ValueString()}
	if !config.StatusMatchesAny.IsNull() && !config.StatusMatchesAny.IsUnknown() {
		resp.Diagnostics.Append(config.StatusMatchesAny.ElementsAs(ctx, &filter.StatusMatchesAny, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	vendors, err := d.client.ListVendors(ctx, filter)
	if err != nil {
		resp.Diagnostics.AddError("Failed to list vendors", err.Error())
		return
	}

	config.Vendors = make([]vendorItemModel, 0, len(vendors))
	for i := range vendors {
		v := &vendors[i]
		config.Vendors = append(config.Vendors, vendorItemModel{
			ID:                types.StringValue(v.ID),
			Name:              types.StringValue(v.Name),
			WebsiteURL:        stringFromPtr(v.WebsiteURL),
			Status:            types.StringValue(v.Status),
			InherentRiskLevel: types.StringValue(v.InherentRiskLevel),
			ResidualRiskLevel: types.StringValue(v.ResidualRiskLevel),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
