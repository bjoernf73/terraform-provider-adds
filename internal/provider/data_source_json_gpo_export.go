package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/bjoernf73/terraform-provider-adds/internal/ad"
	"github.com/bjoernf73/terraform-provider-adds/internal/client"
)

var (
	_ datasource.DataSource              = &jsonGPOExportDataSource{}
	_ datasource.DataSourceWithConfigure = &jsonGPOExportDataSource{}
)

func NewJsonGPOExportDataSource() datasource.DataSource {
	return &jsonGPOExportDataSource{}
}

type jsonGPOExportDataSource struct {
	client *client.Client
}

type jsonGPOExportDataSourceModel struct {
	ID   types.String `tfsdk:"id"`
	Name types.String `tfsdk:"name"`
	JSON types.String `tfsdk:"json"`
}

func (d *jsonGPOExportDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_json_gpo_export"
}

func (d *jsonGPOExportDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads a live GPO's SYSVOL content and renders it as JSON, in the exact shape `adds_json_gpo` " +
			"consumes - the mirror image of that resource: every security principal it finds becomes a portable " +
			"`####Replace[DOMAIN\\Name]` token instead of being resolved from one. Pair with the `local_file` resource from " +
			"the `hashicorp/local` provider to persist the result, the same way `Backup-GPO`'s output isn't itself a " +
			"Terraform concept for `adds_backup_gpo`:\n\n" +
			"```terraform\n" +
			"data \"adds_json_gpo_export\" \"example\" {\n" +
			"  name = \"Domain - GPO5\"\n" +
			"}\n\n" +
			"resource \"local_file\" \"example\" {\n" +
			"  filename = \"${path.module}/json_gpo/Domain - GPO5.json\"\n" +
			"  content  = data.adds_json_gpo_export.example.json\n" +
			"}\n" +
			"```\n\n" +
			"Links, ACLs and WMI filters are not captured, matching `adds_json_gpo`'s import scope.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The GPO's GUID.",
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Display name of the GPO to export.",
			},
			"json": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The exported GPO, as JSON.",
			},
		},
	}
}

func (d *jsonGPOExportDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	addsClient, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("Expected *client.Client, got %T", req.ProviderData))
		return
	}

	d.client = addsClient
}

func (d *jsonGPOExportDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config jsonGPOExportDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	export, err := ad.ExportJsonGPO(ctx, d.client, config.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to export GPO", err.Error())
		return
	}

	if !export.Exists {
		resp.Diagnostics.AddError("GPO not found", fmt.Sprintf("no GPO named %q was found.", config.Name.ValueString()))
		return
	}

	config.ID = types.StringValue(export.GUID)
	config.JSON = types.StringValue(export.JSON)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
