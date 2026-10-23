variable "host" {
  type        = string
  description = "IP address or hostname of the target domain controller."
}

variable "transport" {
  type        = string
  description = "Connection transport: winrm or ssh."
  default     = "winrm"
}

variable "port" {
  type        = number
  description = "Remote port. Null lets the provider pick 5985/5986 for WinRM and 22 for SSH."
  default     = null
  nullable    = true
}

variable "username" {
  type        = string
  description = "Remote account, for example CONTOSO\\Administrator."
}

variable "password" {
  type        = string
  description = "Remote account password."
  sensitive   = true
}

variable "winrm_use_tls" {
  type        = bool
  description = "Use HTTPS for WinRM."
  default     = false
}

variable "winrm_auth" {
  type        = string
  description = "WinRM authentication mechanism: basic, ntlm or kerberos."
  default     = "ntlm"
}

variable "winrm_kerberos_realm" {
  type        = string
  description = "Kerberos realm for WinRM kerberos authentication, for example UTV.LOCAL."
  default     = null
  nullable    = true
}

variable "insecure" {
  type        = bool
  description = "Skip TLS certificate validation for WinRM and host key verification for SSH."
  default     = false
}

variable "powershell_path" {
  type        = string
  description = "PowerShell 7 executable on the target."
  default     = "pwsh"
}

variable "timeout_seconds" {
  type        = number
  description = "Connection timeout in seconds. 120 gives SSH Import-GPO headroom when the single test DC is under load."
  default     = 120
}

variable "ou_path" {
  type        = string
  description = "Slash-delimited OU path created by the smoke test."
  default     = "TerraformCI-ADDS/Smoke"
}

variable "ou_description" {
  type        = string
  description = "Description set on the leaf OU."
  default     = "terraform-provider-adds CI smoke test"
}

variable "group_name" {
  type        = string
  description = "Name of the group created by the smoke test."
  default     = "adds-ci-group"
}

variable "group_scope" {
  type        = string
  description = "Scope of the smoke-test group. The pipeline flips this from Global to DomainLocal on the update apply to exercise the two-step scope conversion through Universal."
  default     = "Global"
}
