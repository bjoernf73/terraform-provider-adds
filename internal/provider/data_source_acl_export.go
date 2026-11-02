package provider

import (
	"context"
	"crypto/sha1"
	"fmt"
	"regexp"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/bjoernf73/terraform-provider-adds/internal/ad"
	"github.com/bjoernf73/terraform-provider-adds/internal/client"
)

var (
	_ datasource.DataSource              = &aclExportDataSource{}
	_ datasource.DataSourceWithConfigure = &aclExportDataSource{}
)

func NewACLExportDataSource() datasource.DataSource {
	return &aclExportDataSource{}
}

type aclExportDataSource struct {
	client *client.Client
}

type aclExportDataSourceModel struct {
	ID           types.String `tfsdk:"id"`
	Path         types.String `tfsdk:"path"`
	Recursive    types.Bool   `tfsdk:"recursive"`
	ObjectTypes  types.Set    `tfsdk:"object_types"`
	IncludeEmpty types.Bool   `tfsdk:"include_empty"`
	Objects      types.List   `tfsdk:"objects"`
	HCL          types.String `tfsdk:"hcl"`
}

func aclExportAceAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"trustee":               types.StringType,
		"access":                types.StringType,
		"rights":                types.ListType{ElemType: types.StringType},
		"object_type":           types.StringType,
		"inherited_object_type": types.StringType,
		"inheritance":           types.StringType,
	}
}

func aclExportObjectAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"distinguished_name":  types.StringType,
		"path":                types.StringType,
		"object_class":        types.StringType,
		"inheritance_blocked": types.BoolType,
		"ace":                 types.ListType{ElemType: types.ObjectType{AttrTypes: aclExportAceAttrTypes()}},
	}
}

func (d *aclExportDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_acl_export"
}

func (d *aclExportDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Exports the explicit (non-inherited) access control entries of one object, or every object " +
			"below a path, as ready-to-use `adds_acl` configuration. Built for lifting an existing domain's delegation into " +
			"Terraform: inherited ACEs are ignored, the **Protect from accidental deletion** ACE is excluded (it is managed " +
			"separately), SIDs and GUIDs are resolved to names, and each object's `inheritance_blocked` state is captured so " +
			"the whole configuration can be replicated.\n\n" +
			"The rendered configuration is exposed as `hcl`; pair it with the `local_file` resource to write a `.tf` file, or " +
			"use `terraform output`:\n\n" +
			"```terraform\n" +
			"data \"adds_acl_export\" \"tree\" {\n" +
			"  path         = \"\" # the domain root\n" +
			"  recursive    = true\n" +
			"  object_types = [\"organizationalUnit\", \"container\"]\n" +
			"}\n\n" +
			"resource \"local_file\" \"acls\" {\n" +
			"  filename = \"${path.module}/imported-acls.tf\"\n" +
			"  content  = data.adds_acl_export.tree.hcl\n" +
			"}\n" +
			"```\n\n" +
			"`target` in the rendered blocks uses the portable form (a slash path for OUs, otherwise a relative distinguished " +
			"name), so an export replays against another domain with the same relative structure. Recursion currently returns " +
			"every object type by default; narrow it with `object_types`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Identifier of this export, derived from the resolved base path and selection.",
			},
			"path": schema.StringAttribute{
				Optional: true,
				MarkdownDescription: "Root object to export. Accepts a slash-delimited path relative to the domain root (for " +
					"example `Contoso/Servers`), a relative or full distinguished name, or `\"\"` (the default) for the domain " +
					"root. The naming-context DNs from `adds_domain` (`configuration_path`, `schema_path`) are accepted too.",
			},
			"recursive": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Export every object below `path` as well as `path` itself. Defaults to `false` (only `path`).",
			},
			"object_types": schema.SetAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Restrict the export to these Active Directory structural class names (`lDAPDisplayName`), for example `[\"organizationalUnit\", \"container\", \"user\"]`. Matching is on the most-specific class, so `user` excludes computers. Omit to export every object type.",
			},
			"include_empty": schema.BoolAttribute{
				Optional: true,
				MarkdownDescription: "Include objects that have no explicit ACEs and do not block inheritance. Defaults to `false`, " +
					"because an object with nothing to replicate would render an empty authoritative `adds_acl` that prunes its DACL on apply.",
			},
			"objects": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "The exported objects, as structured data mirroring the rendered `hcl`.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"distinguished_name": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Full distinguished name of the object.",
						},
						"path": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Portable `target` used in the rendered `adds_acl` block.",
						},
						"object_class": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Structural object class, for example `organizationalUnit`.",
						},
						"inheritance_blocked": schema.BoolAttribute{
							Computed:            true,
							MarkdownDescription: "Whether the object blocks inheritance of ACEs from above.",
						},
						"ace": schema.ListNestedAttribute{
							Computed:            true,
							MarkdownDescription: "The object's explicit, non-inherited ACEs.",
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"trustee":               schema.StringAttribute{Computed: true, MarkdownDescription: "Resolved trustee name, or SID when it cannot be resolved."},
									"access":                schema.StringAttribute{Computed: true, MarkdownDescription: "`Allow` or `Deny`."},
									"rights":                schema.ListAttribute{Computed: true, ElementType: types.StringType, MarkdownDescription: "`ActiveDirectoryRights` values."},
									"object_type":           schema.StringAttribute{Computed: true, MarkdownDescription: "Resolved object type, or empty."},
									"inherited_object_type": schema.StringAttribute{Computed: true, MarkdownDescription: "Resolved inherited object type, or empty."},
									"inheritance":           schema.StringAttribute{Computed: true, MarkdownDescription: "Inheritance flag, or empty for the object only."},
								},
							},
						},
					},
				},
			},
			"hcl": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The exported objects rendered as `adds_acl` configuration, ready to write to a `.tf` file.",
			},
		},
	}
}

func (d *aclExportDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *aclExportDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config aclExportDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var objectTypes []string
	if !config.ObjectTypes.IsNull() && !config.ObjectTypes.IsUnknown() {
		resp.Diagnostics.Append(config.ObjectTypes.ElementsAs(ctx, &objectTypes, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	export, err := ad.ExportACL(ctx, d.client, ad.ACLExportInput{
		Path:         config.Path.ValueString(),
		Recursive:    config.Recursive.ValueBool(),
		ObjectTypes:  objectTypes,
		IncludeEmpty: config.IncludeEmpty.ValueBool(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Unable to export ACLs", err.Error())
		return
	}

	objects, diags := aclExportObjectsToList(ctx, export.Objects)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	config.ID = types.StringValue(aclExportID(config.Path.ValueString(), config.Recursive.ValueBool(), export.Objects))
	config.Objects = objects
	config.HCL = types.StringValue(renderACLExportHCL(export.Objects))

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

func aclExportObjectsToList(ctx context.Context, objects []ad.ACLExportObject) (types.List, diag.Diagnostics) {
	var diags diag.Diagnostics
	objectType := types.ObjectType{AttrTypes: aclExportObjectAttrTypes()}
	aceType := types.ObjectType{AttrTypes: aclExportAceAttrTypes()}

	elems := make([]attr.Value, 0, len(objects))
	for _, object := range objects {
		aces := make([]attr.Value, 0, len(object.ACEs))
		for _, ace := range object.ACEs {
			rights, d := types.ListValueFrom(ctx, types.StringType, ace.Rights)
			diags.Append(d...)

			aceObj, d := types.ObjectValue(aclExportAceAttrTypes(), map[string]attr.Value{
				"trustee":               types.StringValue(ace.Trustee),
				"access":                types.StringValue(ace.Access),
				"rights":                rights,
				"object_type":           types.StringValue(ace.ObjectType),
				"inherited_object_type": types.StringValue(ace.InheritedObjectType),
				"inheritance":           types.StringValue(ace.Inheritance),
			})
			diags.Append(d...)
			aces = append(aces, aceObj)
		}

		aceList, d := types.ListValue(aceType, aces)
		diags.Append(d...)

		objectObj, d := types.ObjectValue(aclExportObjectAttrTypes(), map[string]attr.Value{
			"distinguished_name":  types.StringValue(object.DistinguishedName),
			"path":                types.StringValue(object.Path),
			"object_class":        types.StringValue(object.ObjectClass),
			"inheritance_blocked": types.BoolValue(object.InheritanceBlocked),
			"ace":                 aceList,
		})
		diags.Append(d...)
		elems = append(elems, objectObj)
	}

	list, d := types.ListValue(objectType, elems)
	diags.Append(d...)

	return list, diags
}

func aclExportID(path string, recursive bool, objects []ad.ACLExportObject) string {
	sum := sha1.Sum([]byte(fmt.Sprintf("%s|%t|%d", path, recursive, len(objects))))
	return fmt.Sprintf("%x", sum)
}

var aclExportLabelInvalid = regexp.MustCompile(`[^a-z0-9_]+`)

// renderACLExportHCL turns the exported objects into adds_acl blocks.
func renderACLExportHCL(objects []ad.ACLExportObject) string {
	var b strings.Builder
	used := make(map[string]int, len(objects))

	for i, object := range objects {
		label := aclExportLabel(object, used)
		if i > 0 {
			b.WriteString("\n")
		}

		fmt.Fprintf(&b, "resource \"adds_acl\" %q {\n", label)
		fmt.Fprintf(&b, "  target              = %s\n", hclString(object.Path))
		fmt.Fprintf(&b, "  inheritance_blocked = %t\n", object.InheritanceBlocked)

		for _, ace := range object.ACEs {
			b.WriteString("\n  ace {\n")
			fmt.Fprintf(&b, "    trustee = %s\n", hclString(ace.Trustee))
			if ace.Access != "" {
				fmt.Fprintf(&b, "    access  = %s\n", hclString(ace.Access))
			}
			fmt.Fprintf(&b, "    rights  = %s\n", hclStringList(ace.Rights))
			if ace.ObjectType != "" {
				fmt.Fprintf(&b, "    object_type           = %s\n", hclString(ace.ObjectType))
			}
			if ace.InheritedObjectType != "" {
				fmt.Fprintf(&b, "    inherited_object_type = %s\n", hclString(ace.InheritedObjectType))
			}
			if ace.Inheritance != "" {
				fmt.Fprintf(&b, "    inheritance           = %s\n", hclString(ace.Inheritance))
			}
			b.WriteString("  }\n")
		}

		b.WriteString("}\n")
	}

	return b.String()
}

// aclExportLabel builds a unique, valid Terraform resource label from an object's path (or DN).
func aclExportLabel(object ad.ACLExportObject, used map[string]int) string {
	source := object.Path
	if source == "" {
		source = "domain_root"
	}

	label := aclExportLabelInvalid.ReplaceAllString(strings.ToLower(source), "_")
	label = strings.Trim(label, "_")
	if label == "" || (label[0] >= '0' && label[0] <= '9') {
		label = "acl_" + label
	}
	label = strings.TrimRight(label, "_")

	if n, seen := used[label]; seen {
		used[label] = n + 1
		return fmt.Sprintf("%s_%d", label, n+1)
	}
	used[label] = 1
	return label
}

// hclString renders a value as a double-quoted HCL string, escaping backslashes, quotes and the
// interpolation markers so a trustee such as `CONTOSO\admins` or a value with `${` stays literal.
func hclString(value string) string {
	replacer := strings.NewReplacer(
		`\`, `\\`,
		`"`, `\"`,
		"${", "$${",
		"%{", "%%{",
	)
	return `"` + replacer.Replace(value) + `"`
}

func hclStringList(values []string) string {
	items := make([]string, 0, len(values))
	for _, value := range values {
		items = append(items, hclString(value))
	}
	return "[" + strings.Join(items, ", ") + "]"
}
