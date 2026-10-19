//go:build !windows

package summon

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/cyberark/summon/pkg/secretsyml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetchSecrets_VerifiesProviderBeforeExec(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "ran")
	bin := filepath.Join(t.TempDir(), "prov")
	require.NoError(t, os.WriteFile(bin, []byte("#!/bin/sh\ntouch "+marker+"\n"), 0o755))

	sc := &SubprocessConfig{
		Provider:       bin,
		VerifyProvider: func() error { return errors.New("provider changed") },
		FetchSecret:    func(_ context.Context, _ string) ([]byte, error) { return []byte("v"), nil },
	}
	secrets := secretsyml.SecretsMap{"K": secretsyml.SecretSpec{Tags: []secretsyml.YamlTag{secretsyml.Var}, Path: "p"}}
	tf := NewTempFactory(t.TempDir())
	defer tf.Cleanup()

	_, err := fetchSecrets(context.Background(), secrets, sc, &tf)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "provider changed")
	_, statErr := os.Stat(marker)
	assert.True(t, os.IsNotExist(statErr), "provider must not be exec'd")
}
