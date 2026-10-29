//go:build !solaris

package summon

import (
	"crypto/sha256"
	"os"
	"path/filepath"
	"slices"

	"github.com/cyberark/summon/internal/memzero"
	prov "github.com/cyberark/summon/pkg/provider"
	"github.com/cyberark/summon/pkg/secretsyml"
)

// checksum is the SHA-256 of a secret value.
type checksum = [sha256.Size]byte

// sumOf hashes a secret value through a short-lived copy that is zeroed
// immediately after use.
func sumOf(value string) checksum {
	b := []byte(value)
	defer memzero.Clear(b)
	return sha256.Sum256(b)
}

// checksumResults returns the per-key checksum of every successfully fetched
// env secret. File-typed secrets resolve to a per-cycle temp path, so their
// content is hashed instead of the path.
func checksumResults(results []prov.Result, specs secretsyml.SecretsMap) (map[string]checksum, error) {
	sums := make(map[string]checksum, len(results))
	for _, r := range results {
		if r.Error != nil {
			continue
		}
		spec := specs[r.Key]
		if spec.IsFile() {
			content, err := os.ReadFile(filepath.Clean(r.Value))
			if err != nil {
				return nil, err
			}
			sums[r.Key] = sha256.Sum256(content)
			memzero.Clear(content)
			continue
		}
		sums[r.Key] = sumOf(r.Value)
	}
	return sums, nil
}

// diffKeys returns, sorted, the keys of updated whose checksum differs from
// (or is absent in) old. Keys only present in old are not reported.
func diffKeys(old, updated map[string]checksum) []string {
	var changed []string
	for k, sum := range updated {
		if prev, ok := old[k]; !ok || prev != sum {
			changed = append(changed, k)
		}
	}
	slices.Sort(changed)
	return changed
}
