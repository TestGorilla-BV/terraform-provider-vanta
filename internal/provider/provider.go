package provider

import (
	"context"
	"os"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/TestGorilla-BV/terraform-provider-vanta/internal/client"
)

var _ provider.Provider = (*VantaProvider)(nil)

type VantaProvider struct {
	version string
}

type vantaProviderModel struct {
	ClientID     types.String `tfsdk:"client_id"`
	ClientSecret types.String `tfsdk:"client_secret"`
	Scope        types.String `tfsdk:"scope"`
	Token        types.String `tfsdk:"token"`
	Region       types.String `tfsdk:"region"`
	BaseURL      types.String `tfsdk:"base_url"`
	TokenURL     types.String `tfsdk:"token_url"`

	MaxRequestsPerSecond        types.Float64 `tfsdk:"max_requests_per_second"`
	VendorRiskManagementEnabled types.Bool    `tfsdk:"vendor_risk_management_enabled"`
}

// defaultMaxRequestsPerSecond paces the client so a bulk apply/import stays
// under Vanta's rate limit. Terraform fans resource reads/writes out at its
// configured parallelism (default 10); without pacing that bursts well past
// the API's limit and trips 429s. Tunable via `max_requests_per_second` or the
// VANTA_MAX_REQUESTS_PER_SECOND environment variable.
const defaultMaxRequestsPerSecond = 5.0

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &VantaProvider{version: version}
	}
}

func (p *VantaProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "vanta"
	resp.Version = p.version
}

func (p *VantaProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manage Vanta (https://www.vanta.com) resources via the v1 REST API. " +
			"Authenticates with an OAuth 2.0 client-credentials grant.",
		Attributes: map[string]schema.Attribute{
			"client_id": schema.StringAttribute{
				Description: "OAuth client ID. May be set via the VANTA_CLIENT_ID environment variable. Required unless `token` is set.",
				Optional:    true,
			},
			"client_secret": schema.StringAttribute{
				Description: "OAuth client secret. May be set via the VANTA_CLIENT_SECRET environment variable. Required unless `token` is set.",
				Optional:    true,
				Sensitive:   true,
			},
			"scope": schema.StringAttribute{
				Description: "Space-separated OAuth scopes. Defaults to `vanta-api.all:read vanta-api.all:write`. May be set via VANTA_SCOPE.",
				Optional:    true,
			},
			"token": schema.StringAttribute{
				Description: "A pre-obtained bearer token. When set, the client-credentials exchange is skipped. May be set via VANTA_API_TOKEN.",
				Optional:    true,
				Sensitive:   true,
			},
			"region": schema.StringAttribute{
				Description: "Vanta deployment. One of `us` (commercial) or `gov` (FedRAMP). Defaults to `us`. Ignored when `base_url`/`token_url` are set. May be set via VANTA_REGION.",
				Optional:    true,
				Validators: []validator.String{
					stringvalidator.OneOf("us", "gov"),
				},
			},
			"base_url": schema.StringAttribute{
				Description: "Override the API base URL (e.g. for testing). May be set via VANTA_BASE_URL.",
				Optional:    true,
			},
			"token_url": schema.StringAttribute{
				Description: "Override the OAuth token URL (e.g. for testing). May be set via VANTA_TOKEN_URL.",
				Optional:    true,
			},
			"max_requests_per_second": schema.Float64Attribute{
				Description: "Cap the client's request rate to stay under Vanta's API rate limit during " +
					"bulk applies/imports. Defaults to 5. Set to 0 to disable pacing. May be set via " +
					"VANTA_MAX_REQUESTS_PER_SECOND.",
				Optional: true,
			},
			"vendor_risk_management_enabled": schema.BoolAttribute{
				Description: "Whether the Vanta account has the upgraded Vendor Risk Management add-on. " +
					"When `false` (the default), the `vanta_vendor` resource omits `residual_risk_level`, " +
					"`is_visible_to_auditors`, and custom fields from writes, which the API otherwise " +
					"rejects with a 422 on standard accounts. Enable only if your account has the add-on. " +
					"May be set via VANTA_VENDOR_RISK_MANAGEMENT.",
				Optional: true,
			},
		},
	}
}

func (p *VantaProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var data vantaProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	for name, v := range map[string]types.String{
		"client_id":     data.ClientID,
		"client_secret": data.ClientSecret,
		"token":         data.Token,
	} {
		if v.IsUnknown() {
			resp.Diagnostics.AddAttributeError(path.Root(name), "Unknown "+name, name+" must be known at apply time.")
		}
	}
	if resp.Diagnostics.HasError() {
		return
	}

	clientID := firstNonEmpty(data.ClientID.ValueString(), os.Getenv("VANTA_CLIENT_ID"))
	clientSecret := firstNonEmpty(data.ClientSecret.ValueString(), os.Getenv("VANTA_CLIENT_SECRET"))
	scope := firstNonEmpty(data.Scope.ValueString(), os.Getenv("VANTA_SCOPE"))
	token := firstNonEmpty(data.Token.ValueString(), os.Getenv("VANTA_API_TOKEN"))
	region := firstNonEmpty(data.Region.ValueString(), os.Getenv("VANTA_REGION"))
	baseURL := firstNonEmpty(data.BaseURL.ValueString(), os.Getenv("VANTA_BASE_URL"))
	tokenURL := firstNonEmpty(data.TokenURL.ValueString(), os.Getenv("VANTA_TOKEN_URL"))

	if token == "" && (clientID == "" || clientSecret == "") {
		resp.Diagnostics.AddError(
			"Missing Vanta credentials",
			"Set either `token` (VANTA_API_TOKEN) or both `client_id` (VANTA_CLIENT_ID) and `client_secret` (VANTA_CLIENT_SECRET).",
		)
		return
	}

	maxRPS := resolveMaxRequestsPerSecond(data.MaxRequestsPerSecond, os.Getenv("VANTA_MAX_REQUESTS_PER_SECOND"), resp)
	if resp.Diagnostics.HasError() {
		return
	}
	vrmEnabled := resolveBool(data.VendorRiskManagementEnabled, os.Getenv("VANTA_VENDOR_RISK_MANAGEMENT"))

	c, err := client.New(client.Options{
		ClientID:                    clientID,
		ClientSecret:                clientSecret,
		Scope:                       scope,
		Token:                       token,
		Region:                      region,
		BaseURL:                     baseURL,
		TokenURL:                    tokenURL,
		UserAgent:                   "terraform-provider-vanta/" + p.version,
		RequestsPerSecond:           maxRPS,
		VendorRiskManagementEnabled: vrmEnabled,
	})
	if err != nil {
		resp.Diagnostics.AddError("Failed to construct Vanta client", err.Error())
		return
	}

	resp.DataSourceData = c
	resp.ResourceData = c
}

// resolveMaxRequestsPerSecond picks the request-rate cap from (in order) the
// provider config, the VANTA_MAX_REQUESTS_PER_SECOND environment variable, and
// finally defaultMaxRequestsPerSecond. An explicit 0 (config or env) disables
// pacing; a negative or unparseable value is a configuration error.
func resolveMaxRequestsPerSecond(cfg types.Float64, env string, resp *provider.ConfigureResponse) float64 {
	if !cfg.IsNull() && !cfg.IsUnknown() {
		v := cfg.ValueFloat64()
		if v < 0 {
			resp.Diagnostics.AddAttributeError(path.Root("max_requests_per_second"),
				"Invalid max_requests_per_second", "Must be zero (disable pacing) or positive.")
		}
		return v
	}
	if env != "" {
		v, err := strconv.ParseFloat(env, 64)
		if err != nil || v < 0 {
			resp.Diagnostics.AddError("Invalid VANTA_MAX_REQUESTS_PER_SECOND",
				"Must be a zero or positive number, got "+strconv.Quote(env)+".")
			return 0
		}
		return v
	}
	return defaultMaxRequestsPerSecond
}

// resolveBool reads a boolean from (in order) the provider config and the given
// environment variable, defaulting to false. Any env value strconv.ParseBool
// accepts (1, t, true, ...) enables it; anything else is treated as false.
func resolveBool(cfg types.Bool, env string) bool {
	if !cfg.IsNull() && !cfg.IsUnknown() {
		return cfg.ValueBool()
	}
	v, _ := strconv.ParseBool(env)
	return v
}

func (p *VantaProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewVendorResource,
		NewVendorFindingResource,
	}
}

func (p *VantaProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewVendorsDataSource,
		NewPeopleDataSource,
		NewFrameworksDataSource,
		NewTestsDataSource,
	}
}
