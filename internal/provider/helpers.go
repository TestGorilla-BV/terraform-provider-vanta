package provider

import (
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// UseStateForUnknown plan modifiers. Apply these to every Computed and
// Optional+Computed attribute so that on Update, Terraform treats unset config
// values as "keep what's in state" instead of "(known after apply)". Without
// these, a user who only changes one field sees every other Computed field
// flagged as a planned change.
func stringplanUseStateForUnknown() planmodifier.String {
	return stringplanmodifier.UseStateForUnknown()
}

func boolplanUseStateForUnknown() planmodifier.Bool {
	return boolplanmodifier.UseStateForUnknown()
}

func objectplanUseStateForUnknown() planmodifier.Object {
	return objectplanmodifier.UseStateForUnknown()
}

// firstNonEmpty returns the first non-empty string in vals, or "".
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// stringPtrFromTF returns nil for a null, unknown, or empty Terraform string,
// otherwise a pointer to its value. An empty string is treated as "unset" and
// omitted from the API payload: for the vendor API an empty value is never
// meaningful, and some fields reject it outright — e.g. accountManagerEmail,
// which the server validates as the vendor contact email and rejects "" with a
// 422. This also stops a value the API stores as "" from being echoed back on
// every update.
func stringPtrFromTF(s types.String) *string {
	if s.IsNull() || s.IsUnknown() {
		return nil
	}
	v := s.ValueString()
	if v == "" {
		return nil
	}
	return &v
}

func stringFromPtr(p *string) types.String {
	if p == nil {
		return types.StringNull()
	}
	return types.StringValue(*p)
}

// stringFromEmpty maps "" to null so a field the API reports as empty doesn't
// drift from "plan: null" to "state: empty string".
func stringFromEmpty(s string) types.String {
	if s == "" {
		return types.StringNull()
	}
	return types.StringValue(s)
}

func boolPtrFromTF(b types.Bool) *bool {
	if b.IsNull() || b.IsUnknown() {
		return nil
	}
	v := b.ValueBool()
	return &v
}

func boolFromPtr(p *bool) types.Bool {
	if p == nil {
		return types.BoolNull()
	}
	return types.BoolValue(*p)
}
