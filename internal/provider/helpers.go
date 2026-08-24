package provider

import "github.com/hashicorp/terraform-plugin-framework/types"

// optionalString converts an API *string (nil when absent) into a
// types.String, null when the pointer is nil.
func optionalString(v *string) types.String {
	if v == nil {
		return types.StringNull()
	}
	return types.StringValue(*v)
}

// nullableString converts a types.String into an API *string, nil when the
// value is null or unknown.
func nullableString(v types.String) *string {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	s := v.ValueString()
	return &s
}
