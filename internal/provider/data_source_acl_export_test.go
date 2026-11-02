package provider

import (
	"strings"
	"testing"

	"github.com/bjoernf73/terraform-provider-adds/internal/ad"
)

func TestRenderACLExportHCL(t *testing.T) {
	objects := []ad.ACLExportObject{
		{
			Path:               "Contoso/Servers",
			InheritanceBlocked: true,
			ACEs: []ad.ACLExportAce{
				{
					Trustee: `CONTOSO\Server Admins`,
					Access:  "Allow",
					Rights:  []string{"CreateChild", "DeleteChild"},
				},
				{
					Trustee:             "Authenticated Users",
					Access:              "Deny",
					Rights:              []string{"ExtendedRight"},
					ObjectType:          "Reset Password",
					InheritedObjectType: "user",
					Inheritance:         "Descendents",
				},
			},
		},
	}

	got := renderACLExportHCL(objects)

	for _, want := range []string{
		`resource "adds_acl" "contoso_servers" {`,
		`target              = "Contoso/Servers"`,
		`inheritance_blocked = true`,
		`trustee = "CONTOSO\\Server Admins"`,
		`rights  = ["CreateChild", "DeleteChild"]`,
		`access  = "Deny"`,
		`object_type           = "Reset Password"`,
		`inherited_object_type = "user"`,
		`inheritance           = "Descendents"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered HCL missing %q\n---\n%s", want, got)
		}
	}
}

func TestAclExportLabelDeduplicates(t *testing.T) {
	used := map[string]int{}

	first := aclExportLabel(ad.ACLExportObject{Path: "Contoso/Servers"}, used)
	second := aclExportLabel(ad.ACLExportObject{Path: "Contoso/Servers"}, used)
	root := aclExportLabel(ad.ACLExportObject{Path: ""}, used)

	if first != "contoso_servers" {
		t.Errorf("first label = %q, want contoso_servers", first)
	}
	if second == first {
		t.Errorf("duplicate labels were not disambiguated: both %q", first)
	}
	if root != "domain_root" {
		t.Errorf("root label = %q, want domain_root", root)
	}
}

func TestHCLStringEscaping(t *testing.T) {
	cases := map[string]string{
		`plain`:             `"plain"`,
		`CONTOSO\admins`:    `"CONTOSO\\admins"`,
		`quote"inside`:      `"quote\"inside"`,
		`interp${x}`:        `"interp$${x}"`,
		`template%{if true`: `"template%%{if true"`,
	}

	for input, want := range cases {
		if got := hclString(input); got != want {
			t.Errorf("hclString(%q) = %q, want %q", input, got, want)
		}
	}
}
