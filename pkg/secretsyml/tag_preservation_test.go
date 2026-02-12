package secretsyml

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type tagExpectation struct {
	key  string
	path string
	tag  YamlTag
}

func requireFileSecrets(t *testing.T, config *ParsedConfig, fileIdx int) SecretsMap {
	t.Helper()
	require.Greater(t, len(config.Files), fileIdx)
	secrets, ok := config.Files[fileIdx].Secrets.(SecretsMap)
	require.True(t, ok, "Secrets should be of type SecretsMap")
	return secrets
}

func assertTags(t *testing.T, secrets SecretsMap, expectations []tagExpectation) {
	t.Helper()
	for _, exp := range expectations {
		spec, ok := secrets[exp.key]
		if !assert.True(t, ok, "expected key %q to exist in secrets", exp.key) {
			continue
		}
		assert.Equal(t, exp.path, spec.Path, "%s path", exp.key)
		switch exp.tag {
		case Var:
			assert.True(t, spec.IsVar(), "%s should be !var", exp.key)
		case File:
			assert.True(t, spec.IsFile(), "%s should be !file", exp.key)
		case Literal:
			assert.True(t, spec.IsLiteral(), "%s should be literal", exp.key)
		}
	}
}

func TestYAMLTagPreservation_FileSecrets(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		env          string
		expectations []tagExpectation
	}{
		{
			name: "Simple file secrets with !var tags",
			input: `
summon.files:
  - path: "./output/test.json"
    format: "json"
    secrets:
      DB_USERNAME: !var db/username
      DB_PASSWORD: !var db/password
      API_KEY: !var api/key
`,
			expectations: []tagExpectation{
				{"DB_USERNAME", "db/username", Var},
				{"DB_PASSWORD", "db/password", Var},
				{"API_KEY", "api/key", Var},
			},
		},
		{
			name: "File secrets with environment sections",
			input: `
summon.files:
  - path: "./config/secrets.yaml"
    format: "yaml"
    secrets:
      production:
        DB_HOST: !var prod/db/host
        DB_PORT: !var prod/db/port
      staging:
        DB_HOST: !var staging/db/host
        DB_PORT: !var staging/db/port
`,
			env: "production",
			expectations: []tagExpectation{
				{"DB_HOST", "prod/db/host", Var},
				{"DB_PORT", "prod/db/port", Var},
			},
		},
		{
			name: "Mixed tag types",
			input: `
summon.files:
  - path: "./output/mixed.json"
    format: "json"
    secrets:
      SECRET_KEY: !var secret/key
      CERT_FILE: !file cert/path
      PLAIN_TEXT: !str plain value
      CONFIG_VAR: !var config/setting
`,
			expectations: []tagExpectation{
				{"SECRET_KEY", "secret/key", Var},
				{"CERT_FILE", "cert/path", File},
				{"PLAIN_TEXT", "plain value", Literal},
				{"CONFIG_VAR", "config/setting", Var},
			},
		},
		{
			name: "Common section merging",
			input: `
summon.files:
  - path: "./app/config.json"
    format: "json"
    secrets:
      common:
        COMMON_SECRET: !var common/secret
        COMMON_FILE: !file common/file
      production:
        PROD_SECRET: !var prod/secret
`,
			env: "production",
			expectations: []tagExpectation{
				{"PROD_SECRET", "prod/secret", Var},
				{"COMMON_SECRET", "common/secret", Var},
				{"COMMON_FILE", "common/file", File},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := ParseFromString(tt.input, tt.env, nil)
			require.NoError(t, err)

			secrets := requireFileSecrets(t, config, 0)
			assert.Len(t, secrets, len(tt.expectations))
			assertTags(t, secrets, tt.expectations)
		})
	}
}

func TestYAMLTagPreservation_EnvSecrets(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		env          string
		expectations []tagExpectation
	}{
		{
			name: "Simple env secrets",
			input: `
DATABASE_URL: !var db/connection_string
API_TOKEN: !var api/token
SECRET_KEY: !var app/secret_key
`,
			expectations: []tagExpectation{
				{"DATABASE_URL", "db/connection_string", Var},
				{"API_TOKEN", "api/token", Var},
				{"SECRET_KEY", "app/secret_key", Var},
			},
		},
		{
			name: "With environment sections",
			input: `
production:
  DB_PASSWORD: !var prod/db/password
  API_KEY: !var prod/api/key
staging:
  DB_PASSWORD: !var staging/db/password
  API_KEY: !var staging/api/key
`,
			env: "production",
			expectations: []tagExpectation{
				{"DB_PASSWORD", "prod/db/password", Var},
				{"API_KEY", "prod/api/key", Var},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := ParseFromString(tt.input, tt.env, nil)
			require.NoError(t, err)
			assert.Len(t, config.EnvSecrets, len(tt.expectations))
			assertTags(t, config.EnvSecrets, tt.expectations)
		})
	}
}
