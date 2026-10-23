# GPO permissions are imported with a GPO identity and a trustee identity.
terraform import adds_gpo_permission.server_policy_readers \
  "Servers Baseline|CONTOSO\\Server Policy Readers"
