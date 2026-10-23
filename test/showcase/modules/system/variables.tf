terraform {
  required_providers {
    adds = {
      source = "bjoernf73/adds"
    }
  }
}

variable "system" {
  type        = string
  description = "System short name, 3 to 5 letters (for example \"CRM\")."

  validation {
    condition     = can(regex("^[A-Za-z]{3,5}$", var.system))
    error_message = "system must be 3 to 5 letters."
  }
}

variable "systems_path" {
  type        = string
  description = "Path of the parent \"Systems\" OU the per-system subtree is created under."
}

variable "domain_netbios" {
  type        = string
  description = "Domain NetBIOS name, substituted for the GPO JSON's ####DomainNB#### token."
}

variable "dns_root" {
  type        = string
  description = "Domain DNS root, used to build the gMSA dnsHostName values."
}
