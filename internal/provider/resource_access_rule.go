package provider

import (
	"context"
	"fmt"
	"strings"

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
	_ resource.Resource                   = &accessRuleResource{}
	_ resource.ResourceWithConfigure      = &accessRuleResource{}
	_ resource.ResourceWithImportState    = &accessRuleResource{}
	_ resource.ResourceWithValidateConfig = &accessRuleResource{}
)

func NewAccessRuleResource() resource.Resource {
	return &accessRuleResource{}
}

type accessRuleResource struct {
	client *client.Client
}

type accessRuleResourceModel struct {
	ID                      types.String `tfsdk:"id"`
	Target                  types.String `tfsdk:"target"`
	TargetDN                types.String `tfsdk:"target_dn"`
	Trustee                 types.String `tfsdk:"trustee"`
	Rights                  types.Set    `tfsdk:"rights"`
	Access                  types.String `tfsdk:"access"`
	ObjectType              types.String `tfsdk:"object_type"`
	InheritedObjectType     types.String `tfsdk:"inherited_object_type"`
	Inheritance             types.String `tfsdk:"inheritance"`
	IgnoreAdminCount1       types.Bool   `tfsdk:"ignore_admin_count_1"`
	TrusteeSID              types.String `tfsdk:"trustee_sid"`
	ObjectTypeGUID          types.String `tfsdk:"object_type_guid"`
	InheritedObjectTypeGUID types.String `tfsdk:"inherited_object_type_guid"`
}

func (r *accessRuleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_access_rule"
}

func (r *accessRuleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}

	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a single access control entry (ACE) on an Active Directory object.\n\n" +
			"This resource is **non-authoritative**: it manages only the ACE it declares and never touches " +
			"inherited ACEs, other trustees, or inheritance settings on the target.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Terraform resource identifier, composed of the target, trustee SID, access type, object type GUIDs and inheritance.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"target": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Object the ACE is applied to. Accepts a slash-delimited path relative to the domain root (for example `Servers` or `Contoso/Servers`), a relative distinguished name such as `CN=Computers`, or a full distinguished name. Slash segments are always OU names. The object does not need to be managed by Terraform.",
				PlanModifiers:       replace,
			},
			"trustee": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "Principal the ACE grants or denies rights to. Accepts a SID, a distinguished name, " +
					"`DOMAIN\\name`, a `sAMAccountName`, or a well-known name such as `Authenticated Users`.",
				PlanModifiers: replace,
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
				MarkdownDescription: "`Allow` or `Deny`.",
				Validators: []validator.String{
					oneOf("Allow", "Deny"),
				},
				PlanModifiers: replace,
			},
			"object_type": schema.StringAttribute{
				Optional: true,
				MarkdownDescription: "Object the rights apply to: a schema class (`computer`), an attribute or property set, " +
					"an extended right, a GUID, or `All`. Omit for rights that are not object-specific.",
				PlanModifiers: replace,
			},
			"inherited_object_type": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Schema class of the child objects that inherit this ACE, for example `organizationalUnit`. Requires `inheritance`.",
				PlanModifiers:       replace,
			},
			"inheritance": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "`ActiveDirectorySecurityInheritance`: `None`, `All`, `Descendents`, `SelfAndChildren` or `Children`. Omitting it applies the ACE to the target object only.",
				Validators: []validator.String{
					oneOf("None", "All", "Descendents", "SelfAndChildren", "Children"),
				},
				PlanModifiers: replace,
			},
			"ignore_admin_count_1": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(false),
				MarkdownDescription: "Allow setting this ACE even when `target` has `adminCount = 1` (protected by AdminSDHolder). " +
					"By default this is an error, because SDProp periodically resets the ACL of any adminCount=1 object to " +
					"match AdminSDHolder, silently discarding the ACE this resource just added.",
			},
			"trustee_sid": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Resolved SID of the trustee.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"target_dn": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Resolved distinguished name of `target`.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"object_type_guid": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Resolved GUID of `object_type`.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"inherited_object_type_guid": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Resolved GUID of `inherited_object_type`.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *accessRuleResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config accessRuleResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// .NET has no constructor taking an inherited object type without an inheritance flag.
	if !config.InheritedObjectType.IsNull() && config.Inheritance.IsNull() {
		resp.Diagnostics.AddAttributeError(
			path.Root("inherited_object_type"),
			"Missing inheritance",
			"inherited_object_type requires inheritance to be set.",
		)
	}
}

func (r *accessRuleResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *accessRuleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan accessRuleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	input, diags := accessRuleInput(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	rule, err := ad.EnsureAccessRule(ctx, r.client, input)
	if err != nil {
		resp.Diagnostics.AddError("Unable to create access rule", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, applyAccessRuleResult(plan, rule))...)
}

func (r *accessRuleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state accessRuleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	input, diags := accessRuleInput(ctx, state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	rule, err := ad.ReadAccessRule(ctx, r.client, input)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read access rule", err.Error())
		return
	}

	if !rule.Exists {
		resp.State.RemoveResource(ctx)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, applyAccessRuleResult(state, rule))...)
}

func (r *accessRuleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan accessRuleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	input, diags := accessRuleInput(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	rule, err := ad.EnsureAccessRule(ctx, r.client, input)
	if err != nil {
		resp.Diagnostics.AddError("Unable to update access rule", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, applyAccessRuleResult(plan, rule))...)
}

func (r *accessRuleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state accessRuleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	input, diags := accessRuleInput(ctx, state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := ad.DeleteAccessRule(ctx, r.client, input); err != nil {
		resp.Diagnostics.AddError("Unable to delete access rule", err.Error())
	}
}

func (r *accessRuleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func accessRuleInput(ctx context.Context, model accessRuleResourceModel) (ad.AccessRuleInput, diag.Diagnostics) {
	var rights []string
	diags := model.Rights.ElementsAs(ctx, &rights, false)

	return ad.AccessRuleInput{
		Target:              model.Target.ValueString(),
		Trustee:             model.Trustee.ValueString(),
		Rights:              rights,
		Access:              model.Access.ValueString(),
		ObjectType:          model.ObjectType.ValueString(),
		InheritedObjectType: model.InheritedObjectType.ValueString(),
		Inheritance:         model.Inheritance.ValueString(),
		IgnoreAdminCount1:   model.IgnoreAdminCount1.ValueBool(),
	}, diags
}

// applyAccessRuleResult keeps the configured rights when the effective mask is unchanged,
// because .NET renders some flag combinations under a composite name.
func applyAccessRuleResult(model accessRuleResourceModel, rule *ad.AccessRule) accessRuleResourceModel {
	model.ID = types.StringValue(accessRuleID(rule))
	model.TargetDN = types.StringValue(rule.TargetDN)
	model.TrusteeSID = types.StringValue(rule.TrusteeSID)
	model.ObjectTypeGUID = types.StringValue(rule.ObjectTypeGUID)
	model.InheritedObjectTypeGUID = types.StringValue(rule.InheritedObjectTypeGUID)

	if !rule.RightsMatch {
		rights := make([]attr.Value, 0, len(rule.Rights))
		for _, right := range rule.Rights {
			rights = append(rights, types.StringValue(right))
		}

		model.Rights = types.SetValueMust(types.StringType, rights)
	}

	return model
}

func accessRuleID(rule *ad.AccessRule) string {
	return strings.Join([]string{
		rule.TargetDN,
		rule.TrusteeSID,
		rule.Access,
		rule.ObjectTypeGUID,
		rule.InheritedObjectTypeGUID,
		rule.Inheritance,
	}, "|")
}
