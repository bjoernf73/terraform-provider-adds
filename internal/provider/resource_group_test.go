package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestGroupDistinguishedNameStaysKnownForUnrelatedUpdates(t *testing.T) {
	ctx := context.Background()
	var schemaResp resource.SchemaResponse
	NewGroupResource().Schema(ctx, resource.SchemaRequest{}, &schemaResp)

	prior := groupResourceModel{
		ID:                types.StringValue("c7eb5f92-edb7-40f9-9f77-34a977ff01f1"),
		Name:              types.StringValue("IT-Rolle-Tier1-Operators"),
		SamAccountName:    types.StringValue("IT-Rolle-Tier1-Operators"),
		Path:              types.StringValue("tier1/Groups"),
		ManagedBy:         types.StringValue("IT-Rolle-Tier0-Admins"),
		ManagedByDN:       types.StringValue("CN=IT-Rolle-Tier0-Admins,OU=Groups,OU=tier0,DC=lab,DC=local"),
		ManagerCanUpdate:  types.BoolValue(true),
		Protected:         types.BoolValue(false),
		Category:          types.StringValue("Security"),
		Scope:             types.StringValue("Global"),
		DistinguishedName: types.StringValue("CN=IT-Rolle-Tier1-Operators,OU=Groups,OU=tier1,DC=lab,DC=local"),
		SID:               types.StringValue("S-1-5-21-1-2-3-1173"),
	}

	for _, tc := range []struct {
		name      string
		change    func(*groupResourceModel)
		wantKnown bool
	}{
		{"manager checkbox", func(m *groupResourceModel) { m.ManagerCanUpdate = types.BoolValue(false) }, true},
		{"rename", func(m *groupResourceModel) { m.Name = types.StringValue("IT-Rolle-Tier1-Renamed") }, false},
		{"move", func(m *groupResourceModel) { m.Path = types.StringValue("tier2/Groups") }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			planned := prior
			tc.change(&planned)
			planned.DistinguishedName = types.StringUnknown()

			state := tfsdk.State{Schema: schemaResp.Schema}
			plan := tfsdk.Plan{Schema: schemaResp.Schema}
			if diags := state.Set(ctx, prior); diags.HasError() {
				t.Fatal(diags)
			}
			if diags := plan.Set(ctx, planned); diags.HasError() {
				t.Fatal(diags)
			}

			resp := planmodifier.StringResponse{PlanValue: planned.DistinguishedName}
			useGroupStateUnlessChanged{attributes: []string{"name", "path"}}.PlanModifyString(ctx, planmodifier.StringRequest{
				Plan:       plan,
				State:      state,
				StateValue: prior.DistinguishedName,
				PlanValue:  planned.DistinguishedName,
			}, &resp)

			if resp.Diagnostics.HasError() {
				t.Fatal(resp.Diagnostics)
			}
			if resp.PlanValue.IsUnknown() == tc.wantKnown {
				t.Fatalf("distinguished_name known=%t, want %t", !resp.PlanValue.IsUnknown(), tc.wantKnown)
			}
		})
	}
}
