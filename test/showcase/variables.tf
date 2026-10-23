variable "host" {
  type        = string
  description = "IP address or hostname of the target domain controller."
}

variable "port" {
  type        = number
  description = "WinRM HTTPS port."
  default     = 5986
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

variable "winrm_auth" {
  type        = string
  description = "WinRM authentication mechanism."
  default     = "ntlm"
}

variable "insecure" {
  type        = bool
  description = "Skip TLS certificate validation. Leave false for a CA-issued certificate; set true only for a self-signed listener certificate."
  default     = true
}

variable "powershell_path" {
  type        = string
  description = "PowerShell 7 executable on the target."
  default     = "pwsh"
}

variable "timeout_seconds" {
  type        = number
  description = "Connection timeout in seconds. 120 gives GPO operations headroom when the single test DC is under load."
  default     = 120
}

variable "showcase_path" {
  type        = string
  description = "OU path the showcase objects are created under. Fixed, so repeated runs reconcile rather than accumulate."
  default     = "TerraformCI-ADDS/Showcase"
}
