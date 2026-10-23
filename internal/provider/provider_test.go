package provider

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
)

// testAccProtoV6ProviderFactories serves the in-process provider to the acceptance
// test framework under the name "adds".
var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"adds": providerserver.NewProtocol6WithError(New("test")()),
}

// testAccPreCheck skips acceptance tests unless a target host is configured. Acceptance
// tests are opt-in via TF_ACC and require a reachable domain controller.
func testAccPreCheck(t *testing.T) {
	t.Helper()

	if os.Getenv("ADDS_HOST") == "" {
		t.Skip("ADDS_HOST is not set; skipping acceptance test")
	}

	transport := strings.ToLower(testAccEnv("ADDS_TRANSPORT", "winrm"))
	if os.Getenv("ADDS_USERNAME") == "" {
		t.Fatal("ADDS_USERNAME must be set for acceptance tests")
	}
	if transport == "winrm" && strings.ToLower(testAccEnv("ADDS_WINRM_AUTH", "basic")) != "kerberos" && os.Getenv("ADDS_PASSWORD") == "" {
		t.Fatal("ADDS_PASSWORD must be set for WinRM basic/NTLM acceptance tests")
	}
}

func testAccEnv(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

// testAccProviderConfig renders a provider block from the ADDS_* environment variables so
// acceptance tests target the same host the rest of the tooling uses.
func testAccProviderConfig() string {
	var b strings.Builder
	b.WriteString("provider \"adds\" {\n")
	fmt.Fprintf(&b, "  transport = %q\n", testAccEnv("ADDS_TRANSPORT", "winrm"))
	fmt.Fprintf(&b, "  host      = %q\n", os.Getenv("ADDS_HOST"))
	fmt.Fprintf(&b, "  username  = %q\n", os.Getenv("ADDS_USERNAME"))

	if password := os.Getenv("ADDS_PASSWORD"); password != "" {
		fmt.Fprintf(&b, "  password  = %q\n", password)
	}
	if port := os.Getenv("ADDS_PORT"); port != "" {
		fmt.Fprintf(&b, "  port      = %s\n", port)
	}
	if dc := os.Getenv("ADDS_DOMAIN_CONTROLLER"); dc != "" {
		fmt.Fprintf(&b, "  domain_controller = %q\n", dc)
	}
	if auth := os.Getenv("ADDS_WINRM_AUTH"); auth != "" {
		fmt.Fprintf(&b, "  winrm_auth = %q\n", auth)
	}
	if realm := os.Getenv("ADDS_WINRM_KERBEROS_REALM"); realm != "" {
		fmt.Fprintf(&b, "  winrm_kerberos_realm = %q\n", realm)
	}
	if conf := os.Getenv("ADDS_WINRM_KERBEROS_CONFIG_PATH"); conf != "" {
		fmt.Fprintf(&b, "  winrm_kerberos_config_path = %q\n", conf)
	}
	if spn := os.Getenv("ADDS_WINRM_KERBEROS_SPN"); spn != "" {
		fmt.Fprintf(&b, "  winrm_kerberos_spn = %q\n", spn)
	}
	if ccache := os.Getenv("ADDS_WINRM_KERBEROS_CCACHE_PATH"); ccache != "" {
		fmt.Fprintf(&b, "  winrm_kerberos_ccache_path = %q\n", ccache)
	}
	if testAccEnvBool("ADDS_WINRM_USE_TLS") {
		b.WriteString("  winrm_use_tls = true\n")
	}
	if testAccEnvBool("ADDS_INSECURE") {
		b.WriteString("  insecure = true\n")
	}

	b.WriteString("}\n")
	return b.String()
}

func testAccEnvBool(name string) bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	return value == "1" || value == "true" || value == "yes"
}
