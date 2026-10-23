package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/bjoernf73/terraform-provider-adds/internal/ad"
	"github.com/bjoernf73/terraform-provider-adds/internal/client"
)

var (
	_ resource.Resource                   = &sysvolFilesResource{}
	_ resource.ResourceWithConfigure      = &sysvolFilesResource{}
	_ resource.ResourceWithImportState    = &sysvolFilesResource{}
	_ resource.ResourceWithModifyPlan     = &sysvolFilesResource{}
	_ resource.ResourceWithValidateConfig = &sysvolFilesResource{}
)

func NewNetlogonFilesResource() resource.Resource {
	return &sysvolFilesResource{kind: ad.SysvolTreeNetlogon}
}

func NewAdministrativeTemplatesResource() resource.Resource {
	return &sysvolFilesResource{kind: ad.SysvolTreeAdministrativeTemplates}
}

type sysvolFilesResource struct {
	client *client.Client
	kind   string
}

type sysvolFilesResourceModel struct {
	ID           types.String `tfsdk:"id"`
	SourcePath   types.String `tfsdk:"source_path"`
	Path         types.String `tfsdk:"path"`
	ContentHash  types.String `tfsdk:"content_hash"`
	ManagedPaths types.Set    `tfsdk:"managed_paths"`
}

func (r *sysvolFilesResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	if r.kind == ad.SysvolTreeNetlogon {
		resp.TypeName = req.ProviderTypeName + "_netlogon_files"
		return
	}
	resp.TypeName = req.ProviderTypeName + "_administrative_templates"
}

func (r *sysvolFilesResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attributes := map[string]schema.Attribute{
		"id": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "Terraform identifier for the managed SYSVOL tree.",
			PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		},
		"source_path": schema.StringAttribute{
			Required:            true,
			MarkdownDescription: "Local directory on the machine running Terraform. Every regular file below it is uploaded recursively.",
		},
		"content_hash": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "Fingerprint of source files, recomputed during planning to detect local additions, removals and content changes.",
		},
		"managed_paths": schema.SetAttribute{
			Computed:            true,
			ElementType:         types.StringType,
			MarkdownDescription: "Relative paths currently owned by this resource. Files outside this set are never removed.",
		},
	}

	if r.kind == ad.SysvolTreeNetlogon {
		attributes["path"] = schema.StringAttribute{
			Optional:            true,
			Computed:            true,
			Default:             stringdefault.StaticString(""),
			MarkdownDescription: "Optional relative destination below NETLOGON. Omit to deploy directly into NETLOGON.",
			PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
		}
		resp.Schema = schema.Schema{
			MarkdownDescription: "Recursively manages the files from `source_path` below NETLOGON. It overwrites only declared files and " +
				"removes only paths previously recorded in this resource's state. Empty directories are pruned after file deletion; directories " +
				"containing unrelated files are preserved.",
			Attributes: attributes,
		}
		return
	}

	resp.Schema = schema.Schema{
		MarkdownDescription: "Recursively manages Administrative Template files in the Central Store root, `SYSVOL/<domain>/Policies/PolicyDefinitions`. " +
			"It overwrites only declared files and removes only paths previously recorded in this resource's state. Empty directories are " +
			"pruned after file deletion; directories containing unrelated files are preserved.",
		Attributes: attributes,
	}
}

func (r *sysvolFilesResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	if r.kind != ad.SysvolTreeNetlogon {
		return
	}
	var config sysvolFilesResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() || config.Path.IsNull() || config.Path.IsUnknown() {
		return
	}
	value := strings.ReplaceAll(strings.TrimSpace(config.Path.ValueString()), "\\", "/")
	if strings.HasPrefix(value, "/") || value == ".." || strings.HasPrefix(value, "../") || strings.Contains(value, "/../") {
		resp.Diagnostics.AddAttributeError(path.Root("path"), "Invalid NETLOGON path", "path must be a relative path without `..` segments.")
	}
}

func (r *sysvolFilesResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	var ok bool
	r.client, ok = req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("Expected *client.Client, got %T", req.ProviderData))
	}
}

func (r *sysvolFilesResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	var plan sysvolFilesResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() || plan.SourcePath.IsNull() || plan.SourcePath.IsUnknown() {
		return
	}
	files, err := ad.LoadSysvolFiles(plan.SourcePath.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read SYSVOL source files", err.Error())
		return
	}
	plan.ContentHash = types.StringValue(ad.HashSysvolFiles(files))
	paths := sysvolFilePaths(files)
	plan.ManagedPaths, _ = types.SetValueFrom(ctx, types.StringType, paths)
	resp.Diagnostics.Append(resp.Plan.Set(ctx, &plan)...)
}

func (r *sysvolFilesResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan sysvolFilesResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if !resp.Diagnostics.HasError() {
		r.ensureAndSet(ctx, plan, nil, &resp.State, &resp.Diagnostics)
	}
}

func (r *sysvolFilesResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state sysvolFilesResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	files, err := ad.LoadSysvolFiles(state.SourcePath.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read SYSVOL source files", err.Error())
		return
	}
	result, err := ad.ReadSysvolFiles(ctx, r.client, ad.SysvolFilesInput{Kind: r.kind, RelativePath: state.Path.ValueString(), Files: files})
	if err != nil {
		resp.Diagnostics.AddError("Unable to read managed SYSVOL files", err.Error())
		return
	}
	if !result.Exists {
		resp.State.RemoveResource(ctx)
		return
	}
	if !result.Matches {
		state.ContentHash = types.StringValue("remote-drift")
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *sysvolFilesResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, prior sysvolFilesResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if !resp.Diagnostics.HasError() {
		r.ensureAndSet(ctx, plan, &prior, &resp.State, &resp.Diagnostics)
	}
}

func (r *sysvolFilesResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state sysvolFilesResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	paths, diags := sysvolManagedPaths(ctx, state.ManagedPaths)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := ad.DeleteSysvolFiles(ctx, r.client, ad.SysvolFilesInput{Kind: r.kind, RelativePath: state.Path.ValueString(), PreviousPaths: paths}); err != nil {
		resp.Diagnostics.AddError("Unable to remove managed SYSVOL files", err.Error())
	}
}

func (r *sysvolFilesResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.AddError("Import not supported", "SYSVOL file resources track exactly which files they own. Create the resource from its source directory to establish ownership safely.")
}

func (r *sysvolFilesResource) ensureAndSet(ctx context.Context, plan sysvolFilesResourceModel, prior *sysvolFilesResourceModel, state *tfsdk.State, diags *diag.Diagnostics) {
	files, err := ad.LoadSysvolFiles(plan.SourcePath.ValueString())
	if err != nil {
		diags.AddError("Unable to read SYSVOL source files", err.Error())
		return
	}
	previousPaths := []string(nil)
	if prior != nil {
		previousPaths, *diags = sysvolManagedPaths(ctx, prior.ManagedPaths)
		if diags.HasError() {
			return
		}
	}
	result, err := ad.EnsureSysvolFiles(ctx, r.client, ad.SysvolFilesInput{Kind: r.kind, RelativePath: plan.Path.ValueString(), Files: files, PreviousPaths: previousPaths})
	if err != nil {
		diags.AddError("Unable to manage SYSVOL files", err.Error())
		return
	}
	if !result.Matches {
		diags.AddError("Unable to verify SYSVOL files", "remote files did not match the requested source contents after upload")
		return
	}
	plan.ID = types.StringValue(r.kind + "|" + plan.Path.ValueString())
	plan.ContentHash = types.StringValue(ad.HashSysvolFiles(files))
	plan.ManagedPaths, _ = types.SetValueFrom(ctx, types.StringType, sysvolFilePaths(files))
	diags.Append(state.Set(ctx, &plan)...)
}

func sysvolFilePaths(files []ad.SysvolFile) []string {
	paths := make([]string, 0, len(files))
	for _, file := range files {
		paths = append(paths, file.Path)
	}
	return paths
}

func sysvolManagedPaths(ctx context.Context, paths types.Set) ([]string, diag.Diagnostics) {
	if paths.IsNull() || paths.IsUnknown() {
		return nil, nil
	}
	var values []string
	diags := paths.ElementsAs(ctx, &values, false)
	return values, diags
}
