package ad

import (
	"context"

	"github.com/bjoernf73/terraform-provider-adds/internal/client"
)

const (
	kdsRootKeyCommon = "kds_root_key_common.ps1"
	kdsRootKeyEnsure = "kds_root_key_ensure.ps1"
	kdsRootKeyRead   = "kds_root_key_read.ps1"
)

type KDSRootKey struct {
	KeyID         string `json:"key_id"`
	EffectiveTime string `json:"effective_time"`
	CreationTime  string `json:"creation_time"`
	Created       bool   `json:"created"`
	Exists        bool   `json:"exists"`
}

// EnsureKDSRootKey creates a KDS root key only when none exists, otherwise adopts an existing
// one. effectiveImmediately backdates a newly created key so gMSAs can be used at once, and
// forceReplication pushes a newly created key to every domain controller in the forest.
func EnsureKDSRootKey(ctx context.Context, c *client.Client, effectiveImmediately bool, forceReplication bool) (*KDSRootKey, error) {
	script, err := buildScript(c, map[string]any{
		"effective_immediately": effectiveImmediately,
		"force_replication":     forceReplication,
	}, commonScript, kdsRootKeyCommon, kdsRootKeyEnsure)
	if err != nil {
		return nil, err
	}

	var result KDSRootKey
	if err := c.RunPowerShellJSON(ctx, script, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

func ReadKDSRootKey(ctx context.Context, c *client.Client, keyID string) (*KDSRootKey, error) {
	script, err := buildScript(c, map[string]any{
		"key_id": keyID,
	}, commonScript, kdsRootKeyCommon, kdsRootKeyRead)
	if err != nil {
		return nil, err
	}

	var result KDSRootKey
	if err := c.RunPowerShellJSON(ctx, script, &result); err != nil {
		return nil, err
	}

	return &result, nil
}
