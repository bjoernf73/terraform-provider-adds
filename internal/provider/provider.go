package provider

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	frameworkprovider "github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/bjoernf73/terraform-provider-adds/internal/client"
	"github.com/bjoernf73/terraform-provider-adds/internal/config"
)

var _ frameworkprovider.Provider = &addsProvider{}

func New(version string) func() frameworkprovider.Provider {
	return func() frameworkprovider.Provider {
		return &addsProvider{
			version: version,
		}
	}
}

type addsProvider struct {
	version string
}

type addsProviderModel struct {
	Host                types.String `tfsdk:"host"`
	Port                types.Int64  `tfsdk:"port"`
	Transport           types.String `tfsdk:"transport"`
	Username            types.String `tfsdk:"username"`
	Password            types.String `tfsdk:"password"`
	Insecure            types.Bool   `tfsdk:"insecure"`
	PowerShellPath      types.String `tfsdk:"powershell_path"`
	GPOPowerShellPath   types.String `tfsdk:"gpo_powershell_path"`
	DomainController    types.String `tfsdk:"domain_controller"`
	TimeoutSeconds      types.Int64  `tfsdk:"timeout_seconds"`
	WinRMUseTLS         types.Bool   `tfsdk:"winrm_use_tls"`
	WinRMAuth           types.String `tfsdk:"winrm_auth"`
	WinRMKerberosRealm  types.String `tfsdk:"winrm_kerberos_realm"`
	WinRMKerberosConfig types.String `tfsdk:"winrm_kerberos_config_path"`
	WinRMKerberosSPN    types.String `tfsdk:"winrm_kerberos_spn"`
	WinRMKerberosCCache types.String `tfsdk:"winrm_kerberos_ccache_path"`
	SSHPrivateKeyPEM    types.String `tfsdk:"ssh_private_key_pem"`
	SSHKnownHostsPath   types.String `tfsdk:"ssh_known_hosts_path"`
	SSHHostKey          types.String `tfsdk:"ssh_host_key"`
}

func (p *addsProvider) Metadata(_ context.Context, _ frameworkprovider.MetadataRequest, resp *frameworkprovider.MetadataResponse) {
	resp.TypeName = "adds"
	resp.Version = p.version
}

func (p *addsProvider) Schema(_ context.Context, _ frameworkprovider.SchemaRequest, resp *frameworkprovider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "ADDS is an Active Directory Domain Services provider that manages directory objects and policy through PowerShell 7 commands on remote Windows systems over WinRM or SSH.",
		Attributes: map[string]schema.Attribute{
			"host": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Remote Windows host to connect to.",
			},
			"port": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Remote port. Defaults to 5985 or 5986 for WinRM, and 22 for SSH.",
			},
			"transport": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Connection transport: `winrm` or `ssh`.",
			},
			"username": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Remote account username.",
			},
			"password": schema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				MarkdownDescription: "Remote account password. Used by WinRM and optional for SSH.",
			},
			"insecure": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Skip certificate validation for WinRM TLS or host key validation for SSH.",
			},
			"powershell_path": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "PowerShell 7 executable path on the remote Windows host. Defaults to `pwsh`.",
			},
			"gpo_powershell_path": schema.StringAttribute{
				Optional: true,
				MarkdownDescription: "PowerShell executable for scripts that import the `GroupPolicy` module, including GPO imports, " +
					"the `adds_gpo` data source, JSON/backup exports, permissions, security filtering and WMI-filter assignment. " +
					"Defaults to `powershell.exe`, where the module is native. Set this to `pwsh` to use the Windows PowerShell " +
					"compatibility layer, which returns deserialized objects and can lose nested properties. LDAP-only GPO links " +
					"and backup/JSON resource reads use `powershell_path` instead.",
			},
			"domain_controller": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Optional domain controller to pass to Active Directory cmdlets via `-Server`.",
			},
			"timeout_seconds": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Connection timeout in seconds. Defaults to 30.",
			},
			"winrm_use_tls": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Use HTTPS/TLS for WinRM.",
			},
			"winrm_auth": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "WinRM authentication mechanism: `basic`, `ntlm`, or `kerberos`. Defaults to `basic`.",
			},
			"winrm_kerberos_realm": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Kerberos realm for WinRM Kerberos authentication.",
			},
			"winrm_kerberos_config_path": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Path to `krb5.conf` on the machine running Terraform.",
			},
			"winrm_kerberos_spn": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Optional Kerberos SPN override. Defaults to `HTTP/<host>`.",
			},
			"winrm_kerberos_ccache_path": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Optional Kerberos credential cache path on the machine running Terraform.",
			},
			"ssh_private_key_pem": schema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				MarkdownDescription: "PEM-encoded private key used for SSH authentication.",
			},
			"ssh_known_hosts_path": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Known hosts file for SSH host key verification.",
			},
			"ssh_host_key": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Authorized-key formatted SSH host public key.",
			},
		},
	}
}

func (p *addsProvider) Configure(ctx context.Context, req frameworkprovider.ConfigureRequest, resp *frameworkprovider.ConfigureResponse) {
	var data addsProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	cfg, diags := expandProviderConfig(data)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	addsClient, err := client.New(cfg)
	if err != nil {
		resp.Diagnostics.AddError("Unable to create adds client", err.Error())
		return
	}

	resp.DataSourceData = addsClient
	resp.ResourceData = addsClient
}

func (p *addsProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewOrganizationalUnitResource,
		NewGroupResource,
		NewGroupMemberResource,
		NewACEResource,
		NewAccessRuleResource,
		NewACLResource,
		NewUserResource,
		NewUserPasswordResource,
		NewGMSAResource,
		NewComputerResource,
		NewKDSRootKeyResource,
		NewBackupGPOResource,
		NewJsonGPOResource,
		NewGPOLinksResource,
		NewWMIFilterResource,
		NewGPOWMIFilterResource,
		NewGPOPermissionResource,
		NewGPOSecurityFilterResource,
		NewNetlogonFilesResource,
		NewAdministrativeTemplatesResource,
		NewSiteResource,
		NewSubnetResource,
	}
}

func (p *addsProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewDomainDataSource,
		NewComputerDataSource,
		NewGroupDataSource,
		NewUserDataSource,
		NewOrganizationalUnitDataSource,
		NewGPODataSource,
		NewJsonGPOExportDataSource,
		NewBackupGPOExportDataSource,
	}
}

func expandProviderConfig(data addsProviderModel) (config.Config, diag.Diagnostics) {
	var diags diag.Diagnostics

	cfg := config.Config{
		Host:                strings.TrimSpace(data.Host.ValueString()),
		Transport:           strings.ToLower(strings.TrimSpace(data.Transport.ValueString())),
		Username:            strings.TrimSpace(data.Username.ValueString()),
		Password:            data.Password.ValueString(),
		Insecure:            valueOrDefaultBool(data.Insecure, false),
		PowerShellPath:      valueOrDefaultString(data.PowerShellPath, "pwsh"),
		GPOPowerShellPath:   valueOrDefaultString(data.GPOPowerShellPath, "powershell.exe"),
		DomainController:    strings.TrimSpace(data.DomainController.ValueString()),
		Timeout:             time.Duration(valueOrDefaultInt64(data.TimeoutSeconds, 30)) * time.Second,
		WinRMUseTLS:         valueOrDefaultBool(data.WinRMUseTLS, false),
		WinRMAuth:           strings.ToLower(valueOrDefaultString(data.WinRMAuth, "basic")),
		WinRMKerberosRealm:  strings.TrimSpace(data.WinRMKerberosRealm.ValueString()),
		WinRMKerberosConfig: strings.TrimSpace(data.WinRMKerberosConfig.ValueString()),
		WinRMKerberosSPN:    strings.TrimSpace(data.WinRMKerberosSPN.ValueString()),
		WinRMKerberosCCache: strings.TrimSpace(data.WinRMKerberosCCache.ValueString()),
		SSHPrivateKeyPEM:    data.SSHPrivateKeyPEM.ValueString(),
		SSHKnownHostsPath:   strings.TrimSpace(data.SSHKnownHostsPath.ValueString()),
		SSHHostKey:          strings.TrimSpace(data.SSHHostKey.ValueString()),
	}

	if cfg.Host == "" {
		diags.AddError("Missing host", "host must not be empty.")
	}

	switch cfg.Transport {
	case "winrm":
		if cfg.Username == "" {
			diags.AddError("Missing username", "username is required for WinRM transport.")
		}

		if cfg.WinRMAuth != "kerberos" && cfg.Password == "" {
			diags.AddError("Missing password", "password is required for WinRM basic and NTLM authentication.")
		}

		if cfg.WinRMAuth == "kerberos" && cfg.WinRMKerberosRealm == "" && cfg.WinRMKerberosCCache == "" {
			diags.AddError("Missing Kerberos realm", "winrm_kerberos_realm or winrm_kerberos_ccache_path is required for Kerberos authentication.")
		}

		if data.Port.IsNull() || data.Port.IsUnknown() {
			if cfg.WinRMUseTLS {
				cfg.Port = 5986
			} else {
				cfg.Port = 5985
			}
		} else {
			cfg.Port = int(data.Port.ValueInt64())
		}
	case "ssh":
		if cfg.Username == "" {
			diags.AddError("Missing username", "username is required for SSH transport.")
		}

		if cfg.Password == "" && strings.TrimSpace(cfg.SSHPrivateKeyPEM) == "" {
			diags.AddError("Missing SSH credentials", "password or ssh_private_key_pem is required for SSH transport.")
		}

		if data.Port.IsNull() || data.Port.IsUnknown() {
			cfg.Port = 22
		} else {
			cfg.Port = int(data.Port.ValueInt64())
		}
	default:
		diags.AddError("Unsupported transport", fmt.Sprintf("transport %q is not supported; use `winrm` or `ssh`.", cfg.Transport))
	}

	return cfg, diags
}

func valueOrDefaultString(value types.String, fallback string) string {
	if value.IsNull() || value.IsUnknown() {
		return fallback
	}
	return strings.TrimSpace(value.ValueString())
}

func valueOrDefaultBool(value types.Bool, fallback bool) bool {
	if value.IsNull() || value.IsUnknown() {
		return fallback
	}
	return value.ValueBool()
}

func valueOrDefaultInt64(value types.Int64, fallback int64) int64 {
	if value.IsNull() || value.IsUnknown() {
		return fallback
	}
	return value.ValueInt64()
}
