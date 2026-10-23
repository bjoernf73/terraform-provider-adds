package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/bjoernf73/terraform-provider-adds/internal/ad"
	"github.com/bjoernf73/terraform-provider-adds/internal/client"
	"github.com/bjoernf73/terraform-provider-adds/internal/password"
)

var (
	_ resource.Resource                   = &userPasswordResource{}
	_ resource.ResourceWithConfigure      = &userPasswordResource{}
	_ resource.ResourceWithValidateConfig = &userPasswordResource{}
)

func NewUserPasswordResource() resource.Resource {
	return &userPasswordResource{}
}

type userPasswordResource struct {
	client *client.Client
}

type userPasswordResourceModel struct {
	ID                        types.String `tfsdk:"id"`
	User                      types.String `tfsdk:"user"`
	UserDN                    types.String `tfsdk:"user_dn"`
	Password                  types.String `tfsdk:"password"`
	PasswordWO                types.String `tfsdk:"password_wo"`
	PasswordWOVersion         types.String `tfsdk:"password_wo_version"`
	PasswordLastSet           types.String `tfsdk:"password_last_set"`
	Length                    types.Int64  `tfsdk:"length"`
	ChangePasswordAtNextLogon types.Bool   `tfsdk:"change_password_at_next_logon"`
	Keepers                   types.Map    `tfsdk:"keepers"`
}

func (r *userPasswordResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_user_password"
}

func (r *userPasswordResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replaceString := []planmodifier.String{stringplanmodifier.RequiresReplace()}

	resp.Schema = schema.Schema{
		MarkdownDescription: "Sets the initial password on an Active Directory user account, generating a random " +
			"one unless `password` or `password_wo` is supplied.\n\n" +
			"This resource does not talk to any secret store. To keep a generated password out of your own hands, " +
			"reference this resource's `password` output from a `vault_kv_secret_v2` resource (from the " +
			"`hashicorp/vault` provider) in the same configuration; to reuse a password already stored in " +
			"Vault without it ever touching Terraform state, read it with a `vault_kv_secret_v2` data source and " +
			"pass it into `password_wo` instead.\n\n" +
			"The password can never be read back from Active Directory, so every attribute is immutable: " +
			"changing any of them replaces the resource, generating (or applying) a new password. " +
			"Destroying this resource does not change anything on the account; it only stops Terraform from " +
			"knowing the password.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Terraform resource identifier. Equals the user `objectGUID`.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"user": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "User the password is set on. Accepts a distinguished name, `objectGUID`, SID, " +
					"`DOMAIN\\name` or `sAMAccountName` — typically `adds_user.<name>.id`.",
				PlanModifiers: replaceString,
			},
			"user_dn": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Resolved distinguished name of `user`.",
			},
			"password": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Sensitive:           true,
				MarkdownDescription: "Password to set. Omit (and omit `password_wo`) to generate a random one of `length` characters that satisfies AD's default complexity policy. Stored in state; mutually exclusive with `password_wo`.",
				PlanModifiers:       replaceString,
			},
			"password_wo": schema.StringAttribute{
				Optional:  true,
				Sensitive: true,
				WriteOnly: true,
				MarkdownDescription: "Password to set, supplied out of band (for example from a Vault data source) and never written to plan or state. " +
					"Requires Terraform 1.11+ and must be paired with `password_wo_version`, since Terraform has no stored value to diff a write-only " +
					"attribute against. Mutually exclusive with `password`.",
			},
			"password_wo_version": schema.StringAttribute{
				Optional: true,
				MarkdownDescription: "Arbitrary value paired with `password_wo`. Since the write-only value itself can never be compared against " +
					"state, changing this is what actually triggers replacement (and therefore re-setting the password).",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"password_last_set": schema.StringAttribute{
				Computed: true,
				MarkdownDescription: "AD's `pwdLastSet` for `user`, as an RFC 3339 timestamp. Refreshed on every plan with no replacement " +
					"and no plan modifiers, so if it changes without a corresponding Terraform change, the password was reset by " +
					"someone or something else — visible here, but never acted on automatically.",
			},
			"length": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(24),
				MarkdownDescription: "Length of the generated password. Ignored when `password` is set.",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"change_password_at_next_logon": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(false),
				MarkdownDescription: "Force the user to change the password at their next logon (sets `pwdLastSet` to 0). " +
					"Fire-and-forget: the flag is set when the password is set and is never read back or reconciled, " +
					"because Active Directory clears it once the user changes the password. Leave `false` (the default) " +
					"for automation accounts that keep using the stored password. Cannot be combined with " +
					"`adds_user.password_never_expires` or `cannot_change_password` on the same account.",
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.RequiresReplace(),
				},
			},
			"keepers": schema.MapAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Arbitrary key/value pairs. Changing any value replaces the resource, resetting the password. Use this to trigger deliberate password rotation.",
				PlanModifiers: []planmodifier.Map{
					mapplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

func (r *userPasswordResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *userPasswordResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config userPasswordResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	hasPassword := !config.Password.IsNull() && !config.Password.IsUnknown()
	hasPasswordWO := !config.PasswordWO.IsNull() && !config.PasswordWO.IsUnknown()
	hasVersion := !config.PasswordWOVersion.IsNull() && !config.PasswordWOVersion.IsUnknown()

	if hasPassword && hasPasswordWO {
		resp.Diagnostics.AddAttributeError(
			path.Root("password_wo"),
			"Conflicting password attributes",
			"password and password_wo are mutually exclusive: set at most one of them.",
		)
	}

	if hasPasswordWO && !hasVersion {
		resp.Diagnostics.AddAttributeError(
			path.Root("password_wo_version"),
			"Missing password_wo_version",
			"password_wo requires password_wo_version to also be set, since Terraform cannot detect changes to a write-only value on its own; bump password_wo_version to trigger re-setting the password.",
		)
	}

	if hasVersion && !hasPasswordWO {
		resp.Diagnostics.AddAttributeError(
			path.Root("password_wo"),
			"Missing password_wo",
			"password_wo_version has no effect without password_wo. Use keepers to trigger rotation of a generated password.",
		)
	}
}

func (r *userPasswordResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan userPasswordResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// password_wo is never available via req.Plan (the framework nulls write-only
	// attributes before a plan is persisted); it must be read from the raw config.
	var passwordWO types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("password_wo"), &passwordWO)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var value string
	storePassword := true

	switch {
	case !passwordWO.IsNull() && !passwordWO.IsUnknown():
		value = passwordWO.ValueString()
		storePassword = false
	case !plan.Password.IsNull() && !plan.Password.IsUnknown():
		value = plan.Password.ValueString()
	default:
		generated, err := password.Generate(int(plan.Length.ValueInt64()))
		if err != nil {
			resp.Diagnostics.AddError("Unable to generate password", err.Error())
			return
		}

		value = generated
	}

	target, err := ad.SetUserPassword(ctx, r.client, plan.User.ValueString(), value, plan.ChangePasswordAtNextLogon.ValueBool())
	if err != nil {
		resp.Diagnostics.AddError("Unable to set user password", err.Error())
		return
	}

	plan.ID = types.StringValue(target.UserGUID)
	plan.UserDN = types.StringValue(target.UserDN)
	plan.PasswordLastSet = pwdLastSetValue(target.PwdLastSet)

	if storePassword {
		plan.Password = types.StringValue(value)
	} else {
		plan.Password = types.StringNull()
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *userPasswordResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state userPasswordResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	target, err := ad.ReadUserPasswordTarget(ctx, r.client, state.User.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read user password target", err.Error())
		return
	}

	if !target.Exists {
		resp.State.RemoveResource(ctx)
		return
	}

	// The password itself is left untouched: it cannot be read back from AD, so state
	// is authoritative and is never compared against the live account.
	state.UserDN = types.StringValue(target.UserDN)
	state.PasswordLastSet = pwdLastSetValue(target.PwdLastSet)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update is unreachable: every attribute requires replacement.
func (r *userPasswordResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan userPasswordResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete makes no change on the account: the password and its enabled state are left as
// they are. Destroying this resource only stops Terraform from tracking the password.
func (r *userPasswordResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

// pwdLastSet is empty when AD reports no password has ever been set (pwdLastSet = 0).
func pwdLastSetValue(pwdLastSet string) types.String {
	if pwdLastSet == "" {
		return types.StringNull()
	}

	return types.StringValue(pwdLastSet)
}
