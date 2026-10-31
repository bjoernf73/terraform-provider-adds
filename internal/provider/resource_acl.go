package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/bjoernf73/terraform-provider-adds/internal/ad"
	"github.com/bjoernf73/terraform-provider-adds/internal/client"
)

var (
	_ resource.Resource                   = &aclResource{}
	_ resource.ResourceWithConfigure      = &aclResource{}
	_ resource.ResourceWithImportState    = &aclResource{}
	_ resource.ResourceWithValidateConfig = &aclResource{}
)

func NewACLResource() resource.Resource {
	return &aclResource{}
}

type aclResource struct {
	client *client.Client
}

type aclResourceModel struct {
	ID                 types.String `tfsdk:"id"`
	Target             types.String `tfsdk:"target"`
	TargetDN           types.String `tfsdk:"target_dn"`
	InheritanceBlocked types.Bool   `tfsdk:"inheritance_blocked"`
	IgnoreAdminCount1  types.Bool   `tfsdk:"ignore_admin_count_1"`
	ACEs               types.List   `tfsdk:"ace"`
}

type aclAceModel struct {
	Trustee             types.String `tfsdk:"trustee"`
	Access              types.String `tfsdk:"access"`
	Rights              types.Set    `tfsdk:"rights"`
	ObjectType          types.String `tfsdk:"object_type"`
	InheritedObjectType types.String `tfsdk:"inherited_object_type"`
	Inheritance         types.String `tfsdk:"inheritance"`
}

func aclAceAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"trustee":               types.StringType,
		"access":                types.StringType,
		"rights":                types.SetType{ElemType: types.StringType},
		"object_type":           types.StringType,
		"inherited_object_type": types.StringType,
		"inheritance":           types.StringType,
	}
}

func aclAceObjectType() types.ObjectType {
	return types.ObjectType{AttrTypes: aclAceAttrTypes()}
}

func (r *aclResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_acl"
}

func (r *aclResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the **complete** discretionary ACL (DACL) of an Active Directory object.\n\n" +
			"This resource is **authoritative**: every explicit, non-inherited ACE on `target` that is not " +
			"declared here is removed on apply. ACEs are matched by identity rather than position, so Active " +
			"Directory's own ACE ordering never produces a permanent diff. The **Protect object from accidental " +
			"deletion** ACE (a `Deny` for `Everyone` on `Delete`/`DeleteTree`) and inherited ACEs are always " +
			"preserved and never reported as drift.\n\n" +
			"Because it owns the whole DACL, do not point an `adds_ace` at the same `target`: the next " +
			"apply of this resource would prune it. The recommended way to onboard an existing object is to " +
			"`terraform import` it first, so every current ACE is captured before you edit.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Terraform resource identifier, equal to the resolved distinguished name of `target`.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"target": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Object whose DACL is managed. Accepts a slash-delimited path relative to the domain root (for example `Servers` or `Contoso/Servers`), a relative distinguished name such as `CN=Computers`, or a full distinguished name. Slash segments are always OU names. The object does not need to be managed by Terraform.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"inheritance_blocked": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(false),
				MarkdownDescription: "Break inheritance on `target` so only the declared ACEs (and the accidental-deletion ACE) apply. " +
					"Defaults to `false`, which keeps inheritance enabled: this resource still owns the explicit ACEs but " +
					"leaves inherited ones in place. Set to `true` to break inheritance, in which case inherited ACEs are " +
					"dropped rather than copied down as explicit ACEs. On destroy, inheritance is re-enabled only if this " +
					"resource broke it.",
			},
			"ignore_admin_count_1": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(false),
				MarkdownDescription: "Allow managing the DACL even when `target` has `adminCount = 1` (protected by AdminSDHolder). " +
					"By default this is an error, because SDProp periodically resets the ACL of any adminCount=1 object to " +
					"match AdminSDHolder, silently discarding the ACEs this resource wrote.",
			},
			"target_dn": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Resolved distinguished name of `target`.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"ace": schema.ListNestedAttribute{
				Optional:            true,
				MarkdownDescription: "The complete set of explicit ACEs for `target`. Omit, or set to an empty list, to declare that the object has no explicit ACEs (beyond the always-preserved accidental-deletion ACE).",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"trustee": schema.StringAttribute{
							Required: true,
							MarkdownDescription: "Principal the ACE grants or denies rights to. Accepts a SID, a distinguished name, " +
								"`DOMAIN\\name`, a `sAMAccountName`, or a well-known name such as `Authenticated Users`.",
						},
						"rights": schema.SetAttribute{
							Required:            true,
							ElementType:         types.StringType,
							MarkdownDescription: "`ActiveDirectoryRights` values combined into a single ACE, for example `[\"CreateChild\", \"DeleteChild\"]`.",
						},
						"access": schema.StringAttribute{
							Optional:            true,
							Computed:            true,
							Default:             stringdefault.StaticString("Allow"),
							MarkdownDescription: "`Allow` or `Deny`. Defaults to `Allow`.",
							Validators: []validator.String{
								oneOf("Allow", "Deny"),
							},
						},
						"object_type": schema.StringAttribute{
							Optional: true,
							MarkdownDescription: "Object the rights apply to: a schema class (`computer`), an attribute or property set, " +
								"an extended right, a GUID, or `All`. Omit for rights that are not object-specific.",
						},
						"inherited_object_type": schema.StringAttribute{
							Optional:            true,
							MarkdownDescription: "Schema class of the child objects that inherit this ACE, for example `organizationalUnit`. Requires `inheritance`.",
						},
						"inheritance": schema.StringAttribute{
							Optional:            true,
							MarkdownDescription: "`ActiveDirectorySecurityInheritance`: `None`, `All`, `Descendents`, `SelfAndChildren` or `Children`. Omitting it applies the ACE to the target object only.",
							Validators: []validator.String{
								oneOf("None", "All", "Descendents", "SelfAndChildren", "Children"),
							},
						},
					},
				},
			},
		},
	}
}

func (r *aclResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config aclResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() || config.ACEs.IsNull() || config.ACEs.IsUnknown() {
		return
	}

	var aces []aclAceModel
	resp.Diagnostics.Append(config.ACEs.ElementsAs(ctx, &aces, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	for i, ace := range aces {
		// .NET has no constructor taking an inherited object type without an inheritance flag.
		if !ace.InheritedObjectType.IsNull() && ace.Inheritance.IsNull() {
			resp.Diagnostics.AddAttributeError(
				path.Root("ace").AtListIndex(i).AtName("inherited_object_type"),
				"Missing inheritance",
				"inherited_object_type requires inheritance to be set.",
			)
		}
	}
}

func (r *aclResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	addsClient, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("Expected *client.Client, got %T", req.ProviderData))
		return
	}

	r.client = addsClient
}

func (r *aclResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan aclResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	input, diags := aclInput(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	acl, err := ad.EnsureACL(ctx, r.client, input)
	if err != nil {
		resp.Diagnostics.AddError("Unable to create ACL", err.Error())
		return
	}

	state, diags := applyACLResult(ctx, plan, acl)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *aclResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state aclResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	input, diags := aclInput(ctx, state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	acl, err := ad.ReadACL(ctx, r.client, input)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read ACL", err.Error())
		return
	}

	if !acl.Exists {
		resp.State.RemoveResource(ctx)
		return
	}

	newState, diags := applyACLResult(ctx, state, acl)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, newState)...)
}

func (r *aclResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan aclResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	input, diags := aclInput(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	acl, err := ad.EnsureACL(ctx, r.client, input)
	if err != nil {
		resp.Diagnostics.AddError("Unable to update ACL", err.Error())
		return
	}

	state, diags := applyACLResult(ctx, plan, acl)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *aclResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state aclResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	input, diags := aclInput(ctx, state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := ad.DeleteACL(ctx, r.client, input); err != nil {
		resp.Diagnostics.AddError("Unable to delete ACL", err.Error())
	}
}

func (r *aclResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("target"), req, resp)
}

func aclInput(ctx context.Context, model aclResourceModel) (ad.ACLInput, diag.Diagnostics) {
	var diags diag.Diagnostics

	input := ad.ACLInput{
		Target:             model.Target.ValueString(),
		InheritanceBlocked: model.InheritanceBlocked.ValueBool(),
		IgnoreAdminCount1:  model.IgnoreAdminCount1.ValueBool(),
	}

	if model.ACEs.IsNull() || model.ACEs.IsUnknown() {
		return input, diags
	}

	var aces []aclAceModel
	diags.Append(model.ACEs.ElementsAs(ctx, &aces, false)...)
	if diags.HasError() {
		return input, diags
	}

	for _, ace := range aces {
		var rights []string
		diags.Append(ace.Rights.ElementsAs(ctx, &rights, false)...)

		input.ACEs = append(input.ACEs, ad.ACLAce{
			Trustee:             ace.Trustee.ValueString(),
			Access:              ace.Access.ValueString(),
			Rights:              rights,
			ObjectType:          ace.ObjectType.ValueString(),
			InheritedObjectType: ace.InheritedObjectType.ValueString(),
			Inheritance:         ace.Inheritance.ValueString(),
		})
	}

	return input, diags
}

func applyACLResult(ctx context.Context, model aclResourceModel, acl *ad.ACL) (aclResourceModel, diag.Diagnostics) {
	var diags diag.Diagnostics

	model.ID = types.StringValue(acl.TargetDN)
	model.TargetDN = types.StringValue(acl.TargetDN)
	model.InheritanceBlocked = types.BoolValue(acl.InheritanceBlocked)

	// Preserve a null ace list when there is genuinely nothing to show, so an object managed
	// with no declared ACEs does not flip between null and an empty list.
	if len(acl.ACEs) == 0 && model.ACEs.IsNull() {
		model.ACEs = types.ListNull(aclAceObjectType())
		return model, diags
	}

	list, d := aclAcesToList(ctx, acl.ACEs)
	diags.Append(d...)
	model.ACEs = list

	return model, diags
}

func aclAcesToList(ctx context.Context, aces []ad.ACLAce) (types.List, diag.Diagnostics) {
	var diags diag.Diagnostics

	elems := make([]attr.Value, 0, len(aces))
	for _, ace := range aces {
		rights, d := types.SetValueFrom(ctx, types.StringType, ace.Rights)
		diags.Append(d...)

		obj, d := types.ObjectValue(aclAceAttrTypes(), map[string]attr.Value{
			"trustee":               types.StringValue(ace.Trustee),
			"access":                types.StringValue(ace.Access),
			"rights":                rights,
			"object_type":           aclOptionalString(ace.ObjectType),
			"inherited_object_type": aclOptionalString(ace.InheritedObjectType),
			"inheritance":           aclOptionalString(ace.Inheritance),
		})
		diags.Append(d...)

		elems = append(elems, obj)
	}

	list, d := types.ListValue(aclAceObjectType(), elems)
	diags.Append(d...)

	return list, diags
}

func aclOptionalString(value string) types.String {
	if value == "" {
		return types.StringNull()
	}

	return types.StringValue(value)
}
