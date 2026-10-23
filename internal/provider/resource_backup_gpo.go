package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/bjoernf73/terraform-provider-adds/internal/ad"
	"github.com/bjoernf73/terraform-provider-adds/internal/client"
)

var (
	_ resource.Resource                   = &backupGPOResource{}
	_ resource.ResourceWithConfigure      = &backupGPOResource{}
	_ resource.ResourceWithImportState    = &backupGPOResource{}
	_ resource.ResourceWithModifyPlan     = &backupGPOResource{}
	_ resource.ResourceWithValidateConfig = &backupGPOResource{}
)

func NewBackupGPOResource() resource.Resource {
	return &backupGPOResource{}
}

type backupGPOResource struct {
	client *client.Client
}

type backupGPOMigrationModel struct {
	Source       types.String `tfsdk:"source"`
	Destination  types.String `tfsdk:"destination"`
	SameAsSource types.Bool   `tfsdk:"same_as_source"`
	Type         types.String `tfsdk:"type"`
}

type backupGPOResourceModel struct {
	ID                types.String `tfsdk:"id"`
	BackupName        types.String `tfsdk:"backup_name"`
	Path              types.String `tfsdk:"path"`
	TargetName        types.String `tfsdk:"target_name"`
	Migrations        types.List   `tfsdk:"migrations"`
	ContentHash       types.String `tfsdk:"content_hash"`
	DistinguishedName types.String `tfsdk:"distinguished_name"`
	Domain            types.String `tfsdk:"domain"`
	Status            types.String `tfsdk:"status"`

	// VersionNumber is the AD versionNumber recorded after our last Import-GPO and is the single
	// watermark ModifyPlan diffs against the live value to detect out-of-band edits; Read() must
	// never overwrite it. ComputerADVersion/UserADVersion are its decoded words, readable only.
	VersionNumber     types.Int64 `tfsdk:"version_number"`
	ComputerADVersion types.Int64 `tfsdk:"computer_ad_version"`
	UserADVersion     types.Int64 `tfsdk:"user_ad_version"`
}

func (r *backupGPOResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_backup_gpo"
}

func (r *backupGPOResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Imports a [GPMC backup GPO](https://learn.microsoft.com/en-us/powershell/module/grouppolicy/backup-gpo) " +
			"into Active Directory via `Import-GPO`. This is one of two ways this provider manages GPOs: `adds_backup_gpo` " +
			"imports a folder produced by `Backup-GPO` (or the GPMC UI); `adds_json_gpo` imports a JSON description instead. " +
			"Use `adds_backup_gpo` when you already have (or can export) a working GPO to replicate across domains or " +
			"environments.\n\n" +
			"The backup folder (`path/backup_name/`) is read from the machine running Terraform and uploaded to the target " +
			"Windows host for every apply; the resource has no way to detect out-of-band changes to that folder between plans, " +
			"so it re-imports whenever the folder contents or `migrations` change, detected via a content fingerprint.\n\n" +
			"`Import-GPO` re-imports settings in place when `target_name` already exists, keeping the GPO's GUID and existing " +
			"links, so updates never delete and recreate the GPO.\n\n" +
			"A GPO exposes no content to diff against directly, so drift caused outside Terraform (someone editing the GPO in " +
			"GPMC) is detected through its AD version counter instead: every plan re-checks it against the version " +
			"recorded at the last apply, and re-imports the backup to overwrite the drift when they no longer match. " +
			"`Import-GPO` owns the SYSVOL/GPT.ini version, so this resource tracks only the AD `versionNumber`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Terraform resource identifier. Equals the GPO's GUID.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"backup_name": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "Display name of the backed-up GPO, matching the folder name under `path` and the " +
					"`BackupGpoName`/`DisplayName` recorded in the backup itself (`Import-GPO -BackupGpoName`).",
			},
			"path": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "Local directory, on the machine running Terraform, containing the `backup_name` " +
					"backup folder produced by `Backup-GPO` (one or more `{GUID}` subfolders, `Backup.xml`, `gpreport.xml`, etc).",
			},
			"target_name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Display name of the GPO to create or update in Active Directory.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"migrations": schema.ListNestedAttribute{
				Optional: true,
				MarkdownDescription: "GPO migration table entries, applied by `Import-GPO -MigrationTable` to remap security " +
					"principals and UNC paths baked into the backup (for example when importing into a different domain).",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"source": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: "Value to replace, exactly as it appears in the backup (a SID, `DOMAIN\\name`, or a UNC path). Always reflects the *source* environment the backup was taken from, and does not change when importing into a different target.",
						},
						"destination": schema.StringAttribute{
							Optional:            true,
							MarkdownDescription: "Replacement value. Omit and set `same_as_source` instead to have Import-GPO re-resolve the same name in the target domain/forest rather than substituting a fixed value. Exactly one of `destination` or `same_as_source` is required.",
						},
						"same_as_source": schema.BoolAttribute{
							Optional: true,
							Computed: true,
							Default:  booldefault.StaticBool(false),
							MarkdownDescription: "Re-resolve `source`'s name in the target domain/forest instead of substituting a fixed `destination` " +
								"(GPMC's `<DestinationSameAsSource/>`, what tools like MTEdit emit when no explicit mapping is given). " +
								"Exactly one of `destination` or `same_as_source` is required.",
						},
						"type": schema.StringAttribute{
							Optional: true,
							Computed: true,
							Default:  stringdefault.StaticString("Unknown"),
							MarkdownDescription: "Migration table entry type. One of `User`, `GlobalGroup`, `LocalGroup`, `DomainLocalGroup`, " +
								"`UniversalGroup`, `Computer`, `UNCPath`, `DomainDNSName`, `DomainNetBiosName`, `SidToSid` or `Unknown`. " +
								"Defaults to `Unknown`.",
							Validators: []validator.String{
								oneOf("User", "GlobalGroup", "LocalGroup", "DomainLocalGroup", "UniversalGroup", "Computer", "UNCPath", "DomainDNSName", "DomainNetBiosName", "SidToSid", "Unknown"),
							},
						},
					},
				},
			},
			"content_hash": schema.StringAttribute{
				Computed: true,
				MarkdownDescription: "Fingerprint of the backup folder contents and `migrations`, recomputed from local disk on " +
					"every plan. Not meant to be read directly; it exists so changes to the backup folder are detected even though " +
					"the folder itself is not a Terraform value.",
			},
			"distinguished_name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Distinguished name of the GPO container.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"domain": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Domain the GPO belongs to.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"status": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "GPO status: `AllSettingsEnabled`, `UserSettingsDisabled`, `ComputerSettingsDisabled` or `AllSettingsDisabled`.",
			},
			"version_number": schema.Int64Attribute{
				Computed: true,
				MarkdownDescription: "The GPO's AD `versionNumber`, the single watermark this resource tracks for drift. " +
					"AD increments it on every settings change (by any tool) and never lowers it, so a mismatch against " +
					"the value recorded at the last apply means the GPO was edited outside Terraform; the next plan re-imports " +
					"the backup and records the new, higher version AD assigns rather than forcing the counter back down. It " +
					"packs both side counters: the computer version in the low 16 bits, the user version in the high 16 bits, " +
					"exposed decoded as `computer_ad_version` and `user_ad_version`.",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"computer_ad_version": schema.Int64Attribute{
				Computed: true,
				MarkdownDescription: "Computer-side directory version, the low 16 bits of `version_number`, exposed as a " +
					"human-readable counter. Drift is detected on `version_number`, not this field.",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"user_ad_version": schema.Int64Attribute{
				Computed: true,
				MarkdownDescription: "User-side directory version, the high 16 bits of `version_number`, exposed as a " +
					"human-readable counter. See `version_number`.",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *backupGPOResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config backupGPOResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() || config.Migrations.IsUnknown() || config.Migrations.IsNull() {
		return
	}

	var entries []backupGPOMigrationModel
	resp.Diagnostics.Append(config.Migrations.ElementsAs(ctx, &entries, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	for _, entry := range entries {
		// destination commonly references a not-yet-known value (a data source or
		// another resource's computed attribute), so it can't be judged "unset" from
		// this alone at validate time; defer to apply time instead of false-positiving.
		if entry.Destination.IsUnknown() || entry.SameAsSource.IsUnknown() {
			continue
		}

		hasDestination := !entry.Destination.IsNull() && entry.Destination.ValueString() != ""
		sameAsSource := entry.SameAsSource.ValueBool()

		if hasDestination == sameAsSource {
			resp.Diagnostics.AddAttributeError(
				path.Root("migrations"),
				"Invalid migration entry",
				fmt.Sprintf("migration entry for %q must set exactly one of destination or same_as_source.", entry.Source.ValueString()),
			)
		}
	}
}

func (r *backupGPOResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// ModifyPlan recomputes the local content fingerprint on every plan, so edits to the
// backup folder or migrations are treated as a change even though the folder itself is
// never stored as a Terraform value.
func (r *backupGPOResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return
	}

	var plan backupGPOResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if plan.Path.IsUnknown() || plan.BackupName.IsUnknown() || plan.Migrations.IsUnknown() {
		return
	}

	migrations, diags := backupGPOMigrationsFromList(ctx, plan.Migrations)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	files, err := ad.LoadBackupGPOFiles(plan.Path.ValueString(), plan.BackupName.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read backup GPO folder", err.Error())
		return
	}

	hash := ad.HashBackupGPOContent(files, migrations)
	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("content_hash"), types.StringValue(hash))...)

	r.markVersionsUnknownOnReimport(ctx, req, resp, hash)
}

// markVersionsUnknownOnReimport marks the version attributes unknown whenever this plan will
// re-import the backup - either because the local backup folder/migrations changed (content_hash
// differs) or because the GPO drifted out-of-band since the last apply (live versionNumber moved).
// Import-GPO always bumps the AD versionNumber on re-import and AD never lowers it, so we cannot
// predict the resulting value; leaving the attributes unknown lets Create/Update record whatever
// AD assigns without a "provider produced inconsistent result" error. Read() deliberately never
// touches the watermark (see backupGPOResourceModel), so req.State still holds the last applied
// version to diff against here.
func (r *backupGPOResource) markVersionsUnknownOnReimport(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse, newHash string) {
	if req.State.Raw.IsNull() || r.client == nil {
		return
	}

	var state backupGPOResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	reimport := !state.ContentHash.IsNull() && state.ContentHash.ValueString() != newHash
	if !reimport {
		// Content is unchanged, so only an out-of-band edit can force a re-import; a live read
		// is the only way to see it, since the GPO exposes no content to diff against.
		current, err := ad.ReadBackupGPO(ctx, r.client, state.ID.ValueString())
		if err != nil || !current.Exists {
			// Let the ordinary refresh pass surface the error, or the removal, instead.
			return
		}
		reimport = current.VersionNumber != state.VersionNumber.ValueInt64()
	}
	if !reimport {
		return
	}

	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("version_number"), types.Int64Unknown())...)
	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("computer_ad_version"), types.Int64Unknown())...)
	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("user_ad_version"), types.Int64Unknown())...)
}

func (r *backupGPOResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan backupGPOResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	gpo, err := r.ensure(ctx, plan, resp.Diagnostics)
	if err != nil {
		resp.Diagnostics.AddError("Unable to import backup GPO", err.Error())
		return
	}
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, applyBackupGPOVersion(applyBackupGPOResult(plan, gpo), gpo))...)
}

func (r *backupGPOResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state backupGPOResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	gpo, err := ad.ReadBackupGPO(ctx, r.client, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read backup GPO", err.Error())
		return
	}

	if !gpo.Exists {
		resp.State.RemoveResource(ctx)
		return
	}

	// Deliberately does not touch the version watermark: see markVersionsUnknownOnReimport.
	resp.Diagnostics.Append(resp.State.Set(ctx, applyBackupGPOResult(state, gpo))...)
}

func (r *backupGPOResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan backupGPOResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	gpo, err := r.ensure(ctx, plan, resp.Diagnostics)
	if err != nil {
		resp.Diagnostics.AddError("Unable to import backup GPO", err.Error())
		return
	}
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, applyBackupGPOVersion(applyBackupGPOResult(plan, gpo), gpo))...)
}

func (r *backupGPOResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state backupGPOResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := ad.DeleteBackupGPO(ctx, r.client, state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Unable to delete backup GPO", err.Error())
	}
}

func (r *backupGPOResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// ensure loads the backup folder from local disk and imports it via Import-GPO. Used by
// both Create and Update, since re-running Import-GPO against an existing target GPO
// re-imports settings in place.
func (r *backupGPOResource) ensure(ctx context.Context, plan backupGPOResourceModel, diags diag.Diagnostics) (*ad.BackupGPO, error) {
	migrations, migDiags := backupGPOMigrationsFromList(ctx, plan.Migrations)
	diags.Append(migDiags...)
	if diags.HasError() {
		return nil, nil
	}

	files, err := ad.LoadBackupGPOFiles(plan.Path.ValueString(), plan.BackupName.ValueString())
	if err != nil {
		return nil, err
	}

	return ad.EnsureBackupGPO(ctx, r.client, ad.BackupGPOInput{
		BackupName: plan.BackupName.ValueString(),
		TargetName: plan.TargetName.ValueString(),
		Files:      files,
		Migrations: migrations,
	})
}

func backupGPOMigrationsFromList(ctx context.Context, list types.List) ([]ad.BackupGPOMigration, diag.Diagnostics) {
	if list.IsNull() || list.IsUnknown() {
		return nil, nil
	}

	var models []backupGPOMigrationModel
	diags := list.ElementsAs(ctx, &models, false)
	if diags.HasError() {
		return nil, diags
	}

	migrations := make([]ad.BackupGPOMigration, 0, len(models))
	for _, m := range models {
		migrations = append(migrations, ad.BackupGPOMigration{
			Source:       m.Source.ValueString(),
			Destination:  m.Destination.ValueString(),
			SameAsSource: m.SameAsSource.ValueBool(),
			Type:         m.Type.ValueString(),
		})
	}

	return migrations, diags
}

func applyBackupGPOResult(model backupGPOResourceModel, gpo *ad.BackupGPO) backupGPOResourceModel {
	model.ID = types.StringValue(gpo.GUID)
	model.DistinguishedName = types.StringValue(gpo.DistinguishedName)
	model.Domain = types.StringValue(gpo.Domain)
	model.Status = types.StringValue(gpo.Status)
	return model
}

// applyBackupGPOVersion stamps the version watermark after a successful Import-GPO. Only
// Create/Update should call this; Read() must leave it alone, see markVersionsUnknownOnReimport.
func applyBackupGPOVersion(model backupGPOResourceModel, gpo *ad.BackupGPO) backupGPOResourceModel {
	model.VersionNumber = types.Int64Value(gpo.VersionNumber)
	model.ComputerADVersion = types.Int64Value(gpo.ComputerADVersion)
	model.UserADVersion = types.Int64Value(gpo.UserADVersion)
	return model
}
