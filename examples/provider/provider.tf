terraform {
  required_providers {
    adds = {
      source = "bjoernf73/adds"
    }
  }
}

# WinRM with NTLM authentication.
provider "adds" {
  transport       = "winrm"
  host            = "dc1.contoso.local"
  username        = "CONTOSO\\terraform"
  password        = var.password
  winrm_auth      = "ntlm"
  powershell_path = "pwsh"
}

# SSH with password authentication.
provider "adds" {
  alias           = "ssh"
  transport       = "ssh"
  host            = "dc1.contoso.local"
  username        = "terraform"
  password        = var.password
  powershell_path = "pwsh"
}
