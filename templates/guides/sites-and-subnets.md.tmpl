---
page_title: "Managing Active Directory Sites"
subcategory: "Guides"
description: |-
  Define Active Directory replication sites and map CIDR networks to them for
  site-aware client and domain-controller discovery.
---

# Managing Active Directory Sites

Active Directory sites represent network locations. A client matches its IP address to
an Active Directory subnet, which identifies the site used for domain-controller and
SYSVOL discovery. The usual Terraform shape is one `adds_site` and one or more
`adds_subnet` resources assigned to it.

```hcl
resource "adds_site" "headoffice" {
  name        = "HeadOffice"
  description = "Primary office and data center"
}

resource "adds_subnet" "headoffice_lan" {
  name     = "10.42.0.0/16"
  site     = adds_site.headoffice.name
  location = "Oslo, Norway"
}
```

Subnet names must be CIDR networks with host bits clear. For example, `10.42.0.0/16`
is valid, while `10.42.1.5/16` is rejected.

Changing a site's `description` or a subnet's `site`, `description`, or `location` updates
it in place. Changing a site's name or the CIDR subnet name replaces that object, because
those values are its Active Directory identity.

This provider intentionally manages only site objects and subnet-to-site assignments.
Site links, replication schedules, bridgeheads, and domain-controller placement are
operational topology concerns and are not managed by these resources.
