package ad

import (
	"context"

	"github.com/bjoernf73/terraform-provider-adds/internal/client"
)

const (
	wmiFilterCommon = "wmi_filter_common.ps1"
	wmiFilterEnsure = "wmi_filter_ensure.ps1"
	wmiFilterRead   = "wmi_filter_read.ps1"
	wmiFilterDelete = "wmi_filter_delete.ps1"
)

// WMIFilterQuery is one WQL query in a filter. Multiple queries in the same filter are
// ANDed together by GPMC.
type WMIFilterQuery struct {
	Namespace string
	Query     string
}

type WMIFilterInput struct {
	Name        string
	Description string
	Queries     []WMIFilterQuery
}

func (i WMIFilterInput) payload() map[string]any {
	queries := make([]map[string]any, 0, len(i.Queries))
	for _, q := range i.Queries {
		queries = append(queries, map[string]any{
			"namespace": q.Namespace,
			"query":     q.Query,
		})
	}

	return map[string]any{
		"name":        i.Name,
		"description": i.Description,
		"queries":     queries,
	}
}

type WMIFilterQueryResult struct {
	Namespace string `json:"namespace"`
	Query     string `json:"query"`
}

type WMIFilter struct {
	Exists            bool                   `json:"exists"`
	GUID              string                 `json:"guid"`
	Name              string                 `json:"name"`
	Description       string                 `json:"description"`
	Queries           []WMIFilterQueryResult `json:"queries"`
	DistinguishedName string                 `json:"distinguished_name"`
}

// EnsureWMIFilter gets-or-creates the filter by name and reconciles its description and
// queries. Re-running it is how updates are applied too.
func EnsureWMIFilter(ctx context.Context, c *client.Client, input WMIFilterInput) (*WMIFilter, error) {
	script, err := buildScript(c, input.payload(), commonScript, wmiFilterCommon, wmiFilterEnsure)
	if err != nil {
		return nil, err
	}

	var result WMIFilter
	if err := c.RunPowerShellJSON(ctx, script, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

func ReadWMIFilter(ctx context.Context, c *client.Client, guid string) (*WMIFilter, error) {
	script, err := buildScript(c, map[string]any{
		"guid": guid,
	}, commonScript, wmiFilterCommon, wmiFilterRead)
	if err != nil {
		return nil, err
	}

	var result WMIFilter
	if err := c.RunPowerShellJSON(ctx, script, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

func DeleteWMIFilter(ctx context.Context, c *client.Client, guid string) error {
	script, err := buildScript(c, map[string]any{
		"guid": guid,
	}, commonScript, wmiFilterCommon, wmiFilterDelete)
	if err != nil {
		return err
	}

	var result map[string]any
	return c.RunPowerShellJSON(ctx, script, &result)
}
