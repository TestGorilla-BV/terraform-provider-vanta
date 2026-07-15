package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
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
	// Computed, read-only.
	NextSecurityReviewDueDate        types.String `tfsdk:"next_security_review_due_date"`
	LastSecurityReviewCompletionDate types.String `tfsdk:"last_security_review_completion_date"`
}

func NewVendorResource() resource.Resource {
	return &vendorResource{}
}

func (r *vendorResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vendor"
}

func (r *vendorResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	optString := func(desc string) schema.StringAttribute {
		return schema.StringAttribute{Optional: true, Description: desc}
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
			"website_url":               optString("The vendor's website URL."),
			"account_manager_name":      optString("Name of the external account manager."),
			"account_manager_email":     optString("Email of the external account manager."),
			"services_provided":         optString("Services provided by the vendor."),
			"additional_notes":          optString("Miscellaneous notes about the vendor."),
			"security_owner_user_id":    optString("Vanta user ID of the vendor's security owner."),
			"business_owner_user_id":    optString("Vanta user ID of the vendor's business owner."),
			"contract_start_date":       optString("Contract start date (RFC 3339)."),
			"contract_renewal_date":     optString("Contract renewal date (RFC 3339)."),
			"contract_termination_date": optString("Contract termination date (RFC 3339)."),
			"category":                  optString("The vendor's category."),
			"vendor_headquarters":       optString("ISO 3166-1 alpha-3 country code of the vendor's HQ (e.g. `USA`)."),
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

	v, err := r.client.CreateVendor(ctx, vendorInputFromModel(&plan))
	if err != nil {
		resp.Diagnostics.AddError("Failed to create vendor", err.Error())
		return
	}
	resp.Diagnostics.Append(writeVendorState(ctx, v, &resp.State)...)
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
	resp.Diagnostics.Append(writeVendorState(ctx, v, &resp.State)...)
}

func (r *vendorResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state vendorResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	v, err := r.client.UpdateVendor(ctx, state.ID.ValueString(), vendorInputFromModel(&plan))
	if err != nil {
		resp.Diagnostics.AddError("Failed to update vendor", err.Error())
		return
	}
	resp.Diagnostics.Append(writeVendorState(ctx, v, &resp.State)...)
}

func (r *vendorResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state vendorResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
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

func vendorInputFromModel(m *vendorResourceModel) client.VendorInput {
	return client.VendorInput{
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
		IsVisibleToAuditors:     boolPtrFromTF(m.IsVisibleToAuditors),
		Status:                  stringPtrFromTF(m.Status),
		InherentRiskLevel:       stringPtrFromTF(m.InherentRiskLevel),
		ResidualRiskLevel:       stringPtrFromTF(m.ResidualRiskLevel),
	}
}

func writeVendorState(ctx context.Context, v *client.Vendor, state *tfsdk.State) diag.Diagnostics {
	m := vendorResourceModel{
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
	return state.Set(ctx, &m)
}
