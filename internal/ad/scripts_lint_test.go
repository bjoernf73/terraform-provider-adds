package ad

import (
	"io/fs"
	"strings"
	"testing"
)

// New-Object System.Collections.Generic.List[object] returns an adapter-wrapped list that the
// @(...) array operator rejects on PowerShell 7.6 ("Argument types do not match"), breaking any
// script that later converts the list. Construct List[object] with [..]::new() instead. This guard
// fails fast if the pattern is reintroduced; List[string] and other element types are unaffected.
func TestScriptsAvoidNewObjectListObject(t *testing.T) {
	const forbidden = "New-Object System.Collections.Generic.List[object]"

	err := fs.WalkDir(scripts, "scripts", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".ps1") {
			return nil
		}

		content, err := scripts.ReadFile(path)
		if err != nil {
			return err
		}

		if strings.Contains(string(content), forbidden) {
			t.Errorf("%s uses %q; use [System.Collections.Generic.List[object]]::new() instead "+
				"(New-Object returns an adapter-wrapped list that @() rejects on PowerShell 7.6)", path, forbidden)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking embedded scripts: %v", err)
	}
}
