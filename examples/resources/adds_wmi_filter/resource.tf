resource "adds_wmi_filter" "windows_server_2022" {
  name        = "Windows Server 2022 only"
  description = "Applies GPOs only to Windows Server 2022 machines"

  queries = [
    {
      namespace = "root\\CIMv2"
      query     = "SELECT * FROM Win32_OperatingSystem WHERE Version LIKE \"10.0.20348%\" AND ProductType = \"3\""
    }
  ]
}
