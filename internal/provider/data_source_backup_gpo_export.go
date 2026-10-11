package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/bjoernf73/terraform-provider-adlc/internal/ad"
	"github.com/bjoernf73/terraform-provider-adlc/internal/client"
)

var (
	_ datasource.DataSource              = &backupGPOExportDataSource{}
	_ datasource.DataSourceWithConfigure = &backupGPOExportDataSource{}
)

func NewBackupGPOExportDataSource() datasource.DataSource {
	return &backupGPOExportDataSource{}
}

type backupGPOExportDataSource struct {
	client *client.Client
}

type backupGPOExportDataSourceModel struct {
	ID       types.String `tfsdk:"id"`
	Name     types.String `tfsdk:"name"`
	BackupID types.String `tfsdk:"backup_id"`
	Files    types.Map    `tfsdk:"files"`
}

func (d *backupGPOExportDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_backup_gpo_export"
}

func (d *backupGPOExportDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Exports a live GPO as a native GPMC backup using `Backup-GPO`, regardless of whether the GPO " +
			"was created manually, with `adlc_json_gpo`, or with `adlc_backup_gpo`. Returns every backup file as base64, " +
			"including binary policy files. The provider creates a temporary backup on the remote host and removes it " +
			"after reading the files; it does not write files on the machine running Terraform.\n\n" +
			"Persist `files` with `local_sensitive_file` from the `hashicorp/local` provider using `content_base64`. " +
			"The resulting `<path>/<name>/{backup_id}/...` folder can be used by `adlc_backup_gpo` with `path` pointing " +
			"at its parent and `backup_name` set to `name`. Materialize the files in a separate export apply before " +
			"configuring the import resource, which reads its local backup folder during planning.\n\n" +
			"Every read creates a new snapshot with a new `backup_id` and backup timestamps, even if the policy settings " +
			"have not changed. Uses `gpo_powershell_path` (default `powershell.exe`) and requires the `GroupPolicy` module. " +
			"Backup contents are stored in Terraform state; keep that state secure. This data source does not manage " +
			"GPO links or create an automatic cross-domain migration table.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The source GPO's GUID, not the backup snapshot GUID.",
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Display name of the live GPO to export.",
			},
			"backup_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "GUID of this backup snapshot. A new snapshot is created on every read.",
			},
			"files": schema.MapAttribute{
				Computed:    true,
				Sensitive:   true,
				ElementType: types.StringType,
				MarkdownDescription: "Map of forward-slash-separated relative backup file paths to base64-encoded file contents. " +
					"Snapshot files include the `{backup_id}` directory; root catalog files such as `manifest.xml` keep their root-relative paths. " +
					"Sensitive because a GPO backup can contain credentials or other secrets.",
			},
		},
	}
}

func (d *backupGPOExportDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	adlcClient, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("Expected *client.Client, got %T", req.ProviderData))
		return
	}
	d.client = adlcClient
}

func (d *backupGPOExportDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config backupGPOExportDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	export, err := ad.ExportBackupGPO(ctx, d.client, config.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to export GPO backup", err.Error())
		return
	}
	if !export.Exists {
		resp.Diagnostics.AddError("GPO not found", fmt.Sprintf("no GPO named %q was found.", config.Name.ValueString()))
		return
	}
	files, diags := types.MapValueFrom(ctx, types.StringType, export.Files)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	config.ID = types.StringValue(export.GUID)
	config.BackupID = types.StringValue(export.BackupID)
	config.Files = files
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
