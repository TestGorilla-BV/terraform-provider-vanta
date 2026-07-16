package provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/TestGorilla-BV/terraform-provider-vanta/internal/client"
)

var (
	_ resource.Resource                = (*vendorResource)(nil)
	_ resource.ResourceWithConfigure   = (*vendorResource)(nil)
	_ resource.ResourceWithImportState = (*vendorResource)(nil)
)

// vendorStatuses / riskLevels mirror the API enums.
var (
	vendorStatuses = []string{"MANAGED", "ARCHIVED", "IN_PROCUREMENT"}
	riskLevels     = []string{"CRITICAL", "HIGH", "MEDIUM", "LOW", "UNSCORED"}
	authMethods    = []string{
		"AUTH_0", "AZURE_AD", "GOOGLE_WORKSPACE", "O_AUTH", "O365", "OKTA",
		"ONE_LOGIN", "OWA", "SSO", "USERNAME_PASSWORD", "OTHER",
	}
)

type vendorResource struct {
	client *client.Client
}

type vendorResourceModel struct {
	ID                      types.String `tfsdk:"id"`
	Name                    types.String `tfsdk:"name"`
	WebsiteURL              types.String `tfsdk:"website_url"`
	AccountManagerName      types.String `tfsdk:"account_manager_name"`
	AccountManagerEmail     types.String `tfsdk:"account_manager_email"`
	ServicesProvided        types.String `tfsdk:"services_provided"`
	AdditionalNotes         types.String `tfsdk:"additional_notes"`
	SecurityOwnerUserID     types.String `tfsdk:"security_owner_user_id"`
	BusinessOwnerUserID     types.String `tfsdk:"business_owner_user_id"`
	ContractStartDate       types.String `tfsdk:"contract_start_date"`
	ContractRenewalDate     types.String `tfsdk:"contract_renewal_date"`
	ContractTerminationDate types.String `tfsdk:"contract_termination_date"`
	Category                types.String `tfsdk:"category"`
	VendorHeadquarters      types.String `tfsdk:"vendor_headquarters"`
	IsVisibleToAuditors     types.Bool   `tfsdk:"is_visible_to_auditors"`
	Status                  types.String `tfsdk:"status"`
	InherentRiskLevel       types.String `tfsdk:"inherent_risk_level"`
	ResidualRiskLevel       types.String `tfsdk:"residual_risk_level"`
	AuthenticationMethod    types.String `tfsdk:"authentication_method"`
	// ContractAmount is a types.Object (not a *struct) so it can hold the
	// "unknown" value that Optional+Computed produces on create; a plain struct
	// pointer cannot, and the framework would fail to decode the plan.
	ContractAmount   types.Object `tfsdk:"contract_amount"`
	ArchiveOnDestroy types.Bool   `tfsdk:"archive_on_destroy"`
	AdoptExisting    types.Bool   `tfsdk:"adopt_existing"`
	// Computed, read-only.
	NextSecurityReviewDueDate        types.String `tfsdk:"next_security_review_due_date"`
	LastSecurityReviewCompletionDate types.String `tfsdk:"last_security_review_completion_date"`
}

// contractAmountAttrTypes is the attribute schema of the contract_amount object,
// used to build/decode its types.Object value.
var contractAmountAttrTypes = map[string]attr.Type{
	"amount":   types.Float64Type,
	"currency": types.StringType,
}

func NewVendorResource() resource.Resource {
	return &vendorResource{}
}

func (r *vendorResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vendor"
}

func (r *vendorResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	// optComputedString is for string attributes the API also owns: marking
	// them Optional+Computed means "if config omits it, keep whatever Vanta
	// has" — without Computed, a value Vanta holds but config leaves unset makes
	// Terraform reject the apply as an inconsistent result (planned null vs
	// applied non-null) and churn a perpetual `-> null` diff.
	optComputedString := func(desc string) schema.StringAttribute {
		return schema.StringAttribute{
			Optional:      true,
			Computed:      true,
			Description:   desc,
			PlanModifiers: []planmodifier.String{stringplanUseStateForUnknown()},
		}
	}
	resp.Schema = schema.Schema{
		Description: "A managed third-party vendor. Optional attributes that are removed from " +
			"configuration are not cleared server-side; set them explicitly to overwrite.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				Description:   "Vendor ID.",
				PlanModifiers: []planmodifier.String{stringplanUseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Display name of the vendor.",
			},
			"website_url":               optComputedString("The vendor's website URL."),
			"account_manager_name":      optComputedString("Name of the external account manager."),
			"account_manager_email":     optComputedString("Email of the external account manager."),
			"services_provided":         optComputedString("Services provided by the vendor."),
			"additional_notes":          optComputedString("Miscellaneous notes about the vendor."),
			"security_owner_user_id":    optComputedString("Vanta user ID of the vendor's security owner."),
			"business_owner_user_id":    optComputedString("Vanta user ID of the vendor's business owner."),
			"contract_start_date":       optComputedString("Contract start date (RFC 3339)."),
			"contract_renewal_date":     optComputedString("Contract renewal date (RFC 3339)."),
			"contract_termination_date": optComputedString("Contract termination date (RFC 3339)."),
			"category":                  optComputedString("The vendor's category."),
			"vendor_headquarters":       optComputedString("ISO 3166-1 alpha-3 country code of the vendor's HQ (e.g. `USA`)."),
			"is_visible_to_auditors": schema.BoolAttribute{
				Optional:      true,
				Computed:      true,
				Description:   "Whether auditors can view this vendor.",
				PlanModifiers: []planmodifier.Bool{boolplanUseStateForUnknown()},
			},
			"status": schema.StringAttribute{
				Optional:      true,
				Computed:      true,
				Description:   "Vendor status. One of `MANAGED`, `ARCHIVED`, `IN_PROCUREMENT`.",
				Validators:    []validator.String{stringvalidator.OneOf(vendorStatuses...)},
				PlanModifiers: []planmodifier.String{stringplanUseStateForUnknown()},
			},
			"inherent_risk_level": schema.StringAttribute{
				Optional:      true,
				Computed:      true,
				Description:   "Inherent risk level. One of `CRITICAL`, `HIGH`, `MEDIUM`, `LOW`, `UNSCORED`. Auto-scored by Vanta when unset.",
				Validators:    []validator.String{stringvalidator.OneOf(riskLevels...)},
				PlanModifiers: []planmodifier.String{stringplanUseStateForUnknown()},
			},
			"residual_risk_level": schema.StringAttribute{
				Optional:      true,
				Computed:      true,
				Description:   "Residual risk level. One of `CRITICAL`, `HIGH`, `MEDIUM`, `LOW`, `UNSCORED`.",
				Validators:    []validator.String{stringvalidator.OneOf(riskLevels...)},
				PlanModifiers: []planmodifier.String{stringplanUseStateForUnknown()},
			},
			"authentication_method": schema.StringAttribute{
				Optional: true,
				Computed: true,
				Description: "The vendor's authentication method. One of " +
					"`AUTH_0`, `AZURE_AD`, `GOOGLE_WORKSPACE`, `O_AUTH`, `O365`, `OKTA`, " +
					"`ONE_LOGIN`, `OWA`, `SSO`, `USERNAME_PASSWORD`, `OTHER`.",
				Validators:    []validator.String{stringvalidator.OneOf(authMethods...)},
				PlanModifiers: []planmodifier.String{stringplanUseStateForUnknown()},
			},
			"contract_amount": schema.SingleNestedAttribute{
				Optional:      true,
				Computed:      true,
				Description:   "The vendor's contract amount. When omitted, the vendor's existing amount in Vanta is left untouched.",
				PlanModifiers: []planmodifier.Object{objectplanUseStateForUnknown()},
				Attributes: map[string]schema.Attribute{
					"amount": schema.Float64Attribute{
						Required:    true,
						Description: "The numeric contract amount.",
					},
					"currency": schema.StringAttribute{
						Required:    true,
						Description: "ISO 4217 currency code, e.g. `USD` or `EUR`.",
					},
				},
			},
			"adopt_existing": schema.BoolAttribute{
				Optional: true,
				Description: "When `true`, creating this resource first looks for an existing " +
					"vendor with the same `name`; if exactly one is found it is adopted and " +
					"updated in place instead of creating a duplicate. Use this to bring " +
					"vendors that already exist in Vanta under Terraform management without " +
					"per-resource `import` blocks. An ambiguous name (multiple matches) is an " +
					"error. This attribute is local to Terraform and is not read back from the API.",
			},
			"archive_on_destroy": schema.BoolAttribute{
				Optional: true,
				Description: "When `true`, destroying this resource archives the vendor " +
					"(sets its status to `ARCHIVED`) instead of deleting it from Vanta. " +
					"Defaults to `false` (hard delete). This attribute is local to " +
					"Terraform and is not read back from the API.",
			},
			"next_security_review_due_date": schema.StringAttribute{
				Computed:      true,
				Description:   "The next security review due date.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"last_security_review_completion_date": schema.StringAttribute{
				Computed:      true,
				Description:   "The most recent security review completion date.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *vendorResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("got %T", req.ProviderData))
		return
	}
	r.client = c
}

func (r *vendorResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan vendorResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Adopt an existing same-named vendor instead of creating a duplicate, when
	// requested. This lets callers manage vendors that already exist in Vanta
	// without authoring a per-resource import block (needed on Terraform < 1.7,
	// which lacks for_each in import blocks).
	if plan.AdoptExisting.ValueBool() {
		existing, err := r.client.GetVendorByName(ctx, plan.Name.ValueString())
		switch {
		case err == nil:
			v, err := r.client.UpdateVendor(ctx, existing.ID, vendorInputFromModel(&plan, r.client.VendorRiskManagementEnabled()))
			if err != nil {
				resp.Diagnostics.AddError("Failed to adopt existing vendor", vendorWriteErrorDetail(err))
				return
			}
			resp.Diagnostics.Append(writeVendorState(ctx, v, &plan, &resp.State)...)
			return
		case !client.IsNotFound(err):
			resp.Diagnostics.AddError("Failed to look up existing vendor by name", err.Error())
			return
		}
		// NotFound falls through to a normal create.
	}

	v, err := r.client.CreateVendor(ctx, vendorInputFromModel(&plan, r.client.VendorRiskManagementEnabled()))
	if err != nil {
		resp.Diagnostics.AddError("Failed to create vendor", vendorWriteErrorDetail(err))
		return
	}
	resp.Diagnostics.Append(writeVendorState(ctx, v, &plan, &resp.State)...)
}

func (r *vendorResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state vendorResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	v, err := r.client.GetVendor(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read vendor", err.Error())
		return
	}
	resp.Diagnostics.Append(writeVendorState(ctx, v, &state, &resp.State)...)
}

func (r *vendorResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state vendorResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	v, err := r.client.UpdateVendor(ctx, state.ID.ValueString(), vendorInputFromModel(&plan, r.client.VendorRiskManagementEnabled()))
	if err != nil {
		resp.Diagnostics.AddError("Failed to update vendor", vendorWriteErrorDetail(err))
		return
	}
	resp.Diagnostics.Append(writeVendorState(ctx, v, &plan, &resp.State)...)
}

func (r *vendorResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state vendorResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if state.ArchiveOnDestroy.ValueBool() {
		archived := "ARCHIVED"
		if _, err := r.client.UpdateVendor(ctx, state.ID.ValueString(), client.VendorInput{Status: &archived}); err != nil && !client.IsNotFound(err) {
			resp.Diagnostics.AddError("Failed to archive vendor", err.Error())
		}
		return
	}
	if err := r.client.DeleteVendor(ctx, state.ID.ValueString()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Failed to delete vendor", err.Error())
	}
}

// ImportState accepts either a vendor ID or an exact vendor name.
func (r *vendorResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if req.ID == "" {
		resp.Diagnostics.AddError("Invalid import ID", "expected a vendor ID or a non-empty vendor name")
		return
	}
	// Try a direct ID lookup first; fall back to name.
	if _, err := r.client.GetVendor(ctx, req.ID); err == nil {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
		return
	} else if !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Failed to look up vendor", err.Error())
		return
	}
	v, err := r.client.GetVendorByName(ctx, req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Failed to look up vendor by name", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), v.ID)...)
}

// vendorInputFromModel builds the create/update payload. When vrmEnabled is
// false, the fields Vanta gates behind the upgraded Vendor Risk Management
// add-on (residualRiskLevel, visibleToAuditor) are omitted; sending them on a
// standard account fails the whole write with a 422.
func vendorInputFromModel(m *vendorResourceModel, vrmEnabled bool) client.VendorInput {
	var authDetails *client.VendorAuthDetailsInput
	if method := stringPtrFromTF(m.AuthenticationMethod); method != nil {
		authDetails = &client.VendorAuthDetailsInput{Method: method}
	}
	var contractAmount *client.VendorContractAmount
	if !m.ContractAmount.IsNull() && !m.ContractAmount.IsUnknown() {
		attrs := m.ContractAmount.Attributes()
		amount, _ := attrs["amount"].(types.Float64)
		currency, _ := attrs["currency"].(types.String)
		contractAmount = &client.VendorContractAmount{
			Amount:   amount.ValueFloat64(),
			Currency: currency.ValueString(),
		}
	}
	in := client.VendorInput{
		AuthDetails:             authDetails,
		ContractAmount:          contractAmount,
		Name:                    stringPtrFromTF(m.Name),
		WebsiteURL:              stringPtrFromTF(m.WebsiteURL),
		AccountManagerName:      stringPtrFromTF(m.AccountManagerName),
		AccountManagerEmail:     stringPtrFromTF(m.AccountManagerEmail),
		ServicesProvided:        stringPtrFromTF(m.ServicesProvided),
		AdditionalNotes:         stringPtrFromTF(m.AdditionalNotes),
		SecurityOwnerUserID:     stringPtrFromTF(m.SecurityOwnerUserID),
		BusinessOwnerUserID:     stringPtrFromTF(m.BusinessOwnerUserID),
		ContractStartDate:       stringPtrFromTF(m.ContractStartDate),
		ContractRenewalDate:     stringPtrFromTF(m.ContractRenewalDate),
		ContractTerminationDate: stringPtrFromTF(m.ContractTerminationDate),
		Category:                stringPtrFromTF(m.Category),
		VendorHeadquarters:      stringPtrFromTF(m.VendorHeadquarters),
		Status:                  stringPtrFromTF(m.Status),
		InherentRiskLevel:       stringPtrFromTF(m.InherentRiskLevel),
	}
	if vrmEnabled {
		in.IsVisibleToAuditors = boolPtrFromTF(m.IsVisibleToAuditors)
		in.ResidualRiskLevel = stringPtrFromTF(m.ResidualRiskLevel)
	}
	return in
}

// vendorWriteErrorDetail augments a vendor write failure with a hint when it is
// the 422 Vanta returns for accounts lacking the upgraded Vendor Risk
// Management add-on — the one case where the account's tier, not the config,
// is at fault.
func vendorWriteErrorDetail(err error) string {
	var apiErr *client.APIError
	if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusUnprocessableEntity &&
		strings.Contains(apiErr.Message, "Vendor Risk Management") {
		return err.Error() + "\n\nThis Vanta account does not have the upgraded Vendor Risk Management " +
			"add-on, which is required to set residual_risk_level, is_visible_to_auditors, or custom fields. " +
			"Leave the provider's vendor_risk_management_enabled unset (the default) so these fields are omitted."
	}
	return err.Error()
}

// writeVendorState maps an API vendor onto Terraform state. local carries the
// prior plan/state so the Terraform-only flags (archive_on_destroy,
// adopt_existing), which the API never returns, are preserved across reads.
func writeVendorState(ctx context.Context, v *client.Vendor, local *vendorResourceModel, state *tfsdk.State) diag.Diagnostics {
	var diags diag.Diagnostics
	contractAmount := types.ObjectNull(contractAmountAttrTypes)
	if v.ContractAmount != nil {
		obj, d := types.ObjectValue(contractAmountAttrTypes, map[string]attr.Value{
			"amount":   types.Float64Value(v.ContractAmount.Amount),
			"currency": types.StringValue(v.ContractAmount.Currency),
		})
		diags.Append(d...)
		contractAmount = obj
	}
	m := vendorResourceModel{
		AuthenticationMethod:             stringFromEmpty(v.AuthenticationMethod()),
		ContractAmount:                   contractAmount,
		ArchiveOnDestroy:                 local.ArchiveOnDestroy,
		AdoptExisting:                    local.AdoptExisting,
		ID:                               types.StringValue(v.ID),
		Name:                             types.StringValue(v.Name),
		WebsiteURL:                       stringFromPtr(v.WebsiteURL),
		AccountManagerName:               stringFromPtr(v.AccountManagerName),
		AccountManagerEmail:              stringFromPtr(v.AccountManagerEmail),
		ServicesProvided:                 stringFromPtr(v.ServicesProvided),
		AdditionalNotes:                  stringFromPtr(v.AdditionalNotes),
		SecurityOwnerUserID:              stringFromPtr(v.SecurityOwnerUserID),
		BusinessOwnerUserID:              stringFromPtr(v.BusinessOwnerUserID),
		ContractStartDate:                stringFromPtr(v.ContractStartDate),
		ContractRenewalDate:              stringFromPtr(v.ContractRenewalDate),
		ContractTerminationDate:          stringFromPtr(v.ContractTerminationDate),
		Category:                         stringFromEmpty(v.CategoryDisplayName()),
		VendorHeadquarters:               stringFromPtr(v.VendorHeadquarters),
		IsVisibleToAuditors:              boolFromPtr(v.IsVisibleToAuditors),
		Status:                           types.StringValue(v.Status),
		InherentRiskLevel:                types.StringValue(v.InherentRiskLevel),
		ResidualRiskLevel:                types.StringValue(v.ResidualRiskLevel),
		NextSecurityReviewDueDate:        stringFromPtr(v.NextSecurityReviewDueDate),
		LastSecurityReviewCompletionDate: stringFromPtr(v.LastSecurityReviewCompletionDate),
	}
	diags.Append(state.Set(ctx, &m)...)
	return diags
}
