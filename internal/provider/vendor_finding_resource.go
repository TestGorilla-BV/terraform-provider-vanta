package provider

import (
	"context"
	"fmt"
	"strings"

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
	_ resource.Resource                = (*vendorFindingResource)(nil)
	_ resource.ResourceWithConfigure   = (*vendorFindingResource)(nil)
	_ resource.ResourceWithImportState = (*vendorFindingResource)(nil)
)

var (
	findingRiskStatuses     = []string{"ACCEPT", "REMEDIATE", "NONE"}
	findingRemediationState = []string{"OPEN", "CLOSED"}
)

type vendorFindingResource struct {
	client *client.Client
}

type remediationModel struct {
	State            types.String `tfsdk:"state"`
	RequirementNotes types.String `tfsdk:"requirement_notes"`
}

type vendorFindingResourceModel struct {
	ID               types.String      `tfsdk:"id"`
	VendorID         types.String      `tfsdk:"vendor_id"`
	Content          types.String      `tfsdk:"content"`
	RiskStatus       types.String      `tfsdk:"risk_status"`
	SecurityReviewID types.String      `tfsdk:"security_review_id"`
	DocumentID       types.String      `tfsdk:"document_id"`
	Remediation      *remediationModel `tfsdk:"remediation"`
}

func NewVendorFindingResource() resource.Resource {
	return &vendorFindingResource{}
}

func (r *vendorFindingResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vendor_finding"
}

func (r *vendorFindingResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A finding attached to a vendor. `remediation` is only meaningful when `risk_status` is `REMEDIATE`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				Description:   "Finding ID.",
				PlanModifiers: []planmodifier.String{stringplanUseStateForUnknown()},
			},
			"vendor_id": schema.StringAttribute{
				Required:      true,
				Description:   "ID of the vendor this finding belongs to. Changing this forces replacement.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"content": schema.StringAttribute{
				Required:    true,
				Description: "The content of the finding.",
			},
			"risk_status": schema.StringAttribute{
				Required:    true,
				Description: "Risk status. One of `ACCEPT`, `REMEDIATE`, `NONE`.",
				Validators:  []validator.String{stringvalidator.OneOf(findingRiskStatuses...)},
			},
			"security_review_id": schema.StringAttribute{
				Optional:      true,
				Description:   "ID of a security review to link. Changing this forces replacement.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"document_id": schema.StringAttribute{
				Optional:      true,
				Description:   "ID of a document to link. Changing this forces replacement.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"remediation": schema.SingleNestedAttribute{
				Optional:    true,
				Description: "Remediation details. Populate when `risk_status` is `REMEDIATE`.",
				Attributes: map[string]schema.Attribute{
					"state": schema.StringAttribute{
						Required:    true,
						Description: "Remediation state. One of `OPEN`, `CLOSED`.",
						Validators:  []validator.String{stringvalidator.OneOf(findingRemediationState...)},
					},
					"requirement_notes": schema.StringAttribute{
						Optional:    true,
						Description: "Notes describing what is required to remediate the finding.",
					},
				},
			},
		},
	}
}

func (r *vendorFindingResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *vendorFindingResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan vendorFindingResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	input := client.CreateFindingInput{
		Content:          plan.Content.ValueString(),
		RiskStatus:       plan.RiskStatus.ValueString(),
		SecurityReviewID: stringPtrFromTF(plan.SecurityReviewID),
		DocumentID:       stringPtrFromTF(plan.DocumentID),
		Remediation:      remediationToAPI(plan.Remediation),
	}
	f, err := r.client.CreateVendorFinding(ctx, plan.VendorID.ValueString(), input)
	if err != nil {
		resp.Diagnostics.AddError("Failed to create vendor finding", err.Error())
		return
	}
	resp.Diagnostics.Append(writeVendorFindingState(ctx, f, &resp.State)...)
}

func (r *vendorFindingResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state vendorFindingResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	f, err := r.client.GetVendorFinding(ctx, state.VendorID.ValueString(), state.ID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read vendor finding", err.Error())
		return
	}
	resp.Diagnostics.Append(writeVendorFindingState(ctx, f, &resp.State)...)
}

func (r *vendorFindingResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state vendorFindingResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	content := plan.Content.ValueString()
	riskStatus := plan.RiskStatus.ValueString()
	input := client.UpdateFindingInput{
		Content:     &content,
		RiskStatus:  &riskStatus,
		Remediation: remediationToAPI(plan.Remediation),
	}
	f, err := r.client.UpdateVendorFinding(ctx, state.VendorID.ValueString(), state.ID.ValueString(), input)
	if err != nil {
		resp.Diagnostics.AddError("Failed to update vendor finding", err.Error())
		return
	}
	resp.Diagnostics.Append(writeVendorFindingState(ctx, f, &resp.State)...)
}

func (r *vendorFindingResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state vendorFindingResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteVendorFinding(ctx, state.VendorID.ValueString(), state.ID.ValueString()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Failed to delete vendor finding", err.Error())
	}
}

// ImportState accepts a composite "<vendor_id>/<finding_id>" identifier.
func (r *vendorFindingResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			fmt.Sprintf("expected \"<vendor_id>/<finding_id>\", got %q", req.ID),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("vendor_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[1])...)
}

func remediationToAPI(m *remediationModel) *client.Remediation {
	if m == nil {
		return nil
	}
	return &client.Remediation{
		State:            m.State.ValueString(),
		RequirementNotes: stringPtrFromTF(m.RequirementNotes),
	}
}

func writeVendorFindingState(ctx context.Context, f *client.VendorFinding, state *tfsdk.State) diag.Diagnostics {
	m := vendorFindingResourceModel{
		ID:               types.StringValue(f.ID),
		VendorID:         types.StringValue(f.VendorID),
		Content:          types.StringValue(f.Content),
		RiskStatus:       types.StringValue(f.RiskStatus),
		SecurityReviewID: stringFromPtr(f.SecurityReviewID),
		DocumentID:       stringFromPtr(f.DocumentID),
	}
	if f.Remediation != nil {
		m.Remediation = &remediationModel{
			State:            types.StringValue(f.Remediation.State),
			RequirementNotes: stringFromPtr(f.Remediation.RequirementNotes),
		}
	}
	return state.Set(ctx, &m)
}
