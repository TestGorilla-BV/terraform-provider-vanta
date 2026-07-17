package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TestVendorInputOmitsRiskManagementFieldsWhenDisabled verifies that the
// Vendor-Risk-Management-gated fields (residualRiskLevel, visibleToAuditor) are
// only sent when the account has the add-on. Sending them on a standard account
// makes Vanta reject the whole write with a 422.
func TestVendorInputOmitsRiskManagementFieldsWhenDisabled(t *testing.T) {
	m := &vendorResourceModel{
		Name:                types.StringValue("Acme Inc"),
		InherentRiskLevel:   types.StringValue("LOW"),
		ResidualRiskLevel:   types.StringValue("UNSCORED"),
		IsVisibleToAuditors: types.BoolValue(true),
	}

	disabled := vendorInputFromModel(m, false)
	if disabled.ResidualRiskLevel != nil {
		t.Errorf("residualRiskLevel should be omitted when VRM disabled, got %q", *disabled.ResidualRiskLevel)
	}
	if disabled.IsVisibleToAuditors != nil {
		t.Errorf("isVisibleToAuditors should be omitted when VRM disabled, got %v", *disabled.IsVisibleToAuditors)
	}
	// Non-gated fields are always sent.
	if disabled.Name == nil || *disabled.Name != "Acme Inc" {
		t.Errorf("name should always be sent, got %v", disabled.Name)
	}
	if disabled.InherentRiskLevel == nil || *disabled.InherentRiskLevel != "LOW" {
		t.Errorf("inherentRiskLevel is not VRM-gated and should be sent, got %v", disabled.InherentRiskLevel)
	}

	enabled := vendorInputFromModel(m, true)
	if enabled.ResidualRiskLevel == nil || *enabled.ResidualRiskLevel != "UNSCORED" {
		t.Errorf("residualRiskLevel should be sent when VRM enabled, got %v", enabled.ResidualRiskLevel)
	}
	if enabled.IsVisibleToAuditors == nil || *enabled.IsVisibleToAuditors != true {
		t.Errorf("isVisibleToAuditors should be sent when VRM enabled, got %v", enabled.IsVisibleToAuditors)
	}
}

// TestVendorInputOmitsEmptyStrings verifies that an empty-string optional field
// is omitted from the payload rather than sent as "". Vanta stores some fields
// (e.g. accountManagerEmail) as "" and validates them as an email on write,
// rejecting "" with a 422; the empty value must not be echoed back.
func TestVendorInputOmitsEmptyStrings(t *testing.T) {
	m := &vendorResourceModel{
		Name:                types.StringValue("Acme Inc"),
		AccountManagerEmail: types.StringValue(""), // Vanta returned "" on read
		WebsiteURL:          types.StringValue("https://acme.example"),
	}
	in := vendorInputFromModel(m, false)
	if in.AccountManagerEmail != nil {
		t.Errorf("empty accountManagerEmail should be omitted, got %q", *in.AccountManagerEmail)
	}
	if in.WebsiteURL == nil || *in.WebsiteURL != "https://acme.example" {
		t.Errorf("non-empty websiteUrl should be sent, got %v", in.WebsiteURL)
	}
}
