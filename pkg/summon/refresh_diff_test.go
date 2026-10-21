//go:build !solaris

package summon

import (
	"os"
	"path/filepath"
	"testing"

	prov "github.com/cyberark/summon/pkg/provider"
	"github.com/cyberark/summon/pkg/secretsyml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiffKeys(t *testing.T) {
	sum := func(s string) checksum { return sumOf(s) }
	old := map[string]checksum{"A": sum("1"), "B": sum("2"), "GONE": sum("x")}
	updated := map[string]checksum{"A": sum("1"), "B": sum("changed"), "NEW": sum("n")}

	assert.Equal(t, []string{"B", "NEW"}, diffKeys(old, updated))
	assert.Empty(t, diffKeys(updated, updated))
}

func TestChecksumResults(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "f")
	require.NoError(t, os.WriteFile(filePath, []byte("file-content"), 0o600))
	specs := secretsyml.SecretsMap{
		"PLAIN": {Tags: nil, Path: "p"},
		"FILE":  {Tags: []secretsyml.YamlTag{secretsyml.File}, Path: "q"},
	}
	results := []prov.Result{
		{Key: "PLAIN", Value: "v1"},
		{Key: "FILE", Value: filePath},
		{Key: "BROKEN", Error: assert.AnError},
	}

	sums, err := checksumResults(results, specs)

	require.NoError(t, err)
	assert.Equal(t, sumOf("v1"), sums["PLAIN"])
	// file-typed secrets hash the content, not the (per-cycle) temp path
	assert.Equal(t, sumOf("file-content"), sums["FILE"])
	assert.NotContains(t, sums, "BROKEN")
}

// TestChecksumResults_CleanedPath verifies that a !file path with redundant
// segments is cleaned before ReadFile so the correct content is hashed
// (Snyk CWE-22).
func TestChecksumResults_CleanedPath(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "f")
	require.NoError(t, os.WriteFile(filePath, []byte("clean-content"), 0o600))

	specs := secretsyml.SecretsMap{
		"FILE": {Tags: []secretsyml.YamlTag{secretsyml.File}, Path: "q"},
	}
	// Supply a path with a redundant ./subdir/../ that resolves to filePath.
	dirty := dir + "/subdir/../f"
	results := []prov.Result{{Key: "FILE", Value: dirty}}

	sums, err := checksumResults(results, specs)

	require.NoError(t, err)
	assert.Equal(t, sumOf("clean-content"), sums["FILE"])
}
