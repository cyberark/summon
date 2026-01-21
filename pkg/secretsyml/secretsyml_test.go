package secretsyml

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

type testCase struct {
	name         string
	defaultValue string
	path         string
	isVar        bool
	isFile       bool
	isLiteral    bool
}

func TestParseFromString(t *testing.T) {
	testEnv := ""
	t.Run("Given a string in secrets.yml format", func(t *testing.T) {
		input := `
SENTRY_API_KEY: !var $env/sentry/api_key
PRIVATE_KEY_FILE: !file:var $env/aws/ec2/private_key
PRIVATE_KEY_FILE2: !var:file $env/aws/ec2/private_key
SOME_FILE: !file my content
RAILS_ENV: $env
DEFAULT_VAR: !var:default='defaultvalue':file $env/sentry/api_key
DEFAULT_VAR2: !default='def' foo
SOME_ESCAPING_VAR: FOO$$BAR
FLOAT: 27.1111
INT: 27
BOOL: true`

		testCases := []testCase{
			{
				name:      "SENTRY_API_KEY",
				isVar:     true,
				isFile:    false,
				isLiteral: false,
			},
			{
				name:      "PRIVATE_KEY_FILE",
				isVar:     true,
				isFile:    true,
				isLiteral: false,
			},
			{
				name:      "PRIVATE_KEY_FILE2",
				isVar:     true,
				isFile:    true,
				isLiteral: false,
			},
			{
				name:      "SOME_FILE",
				isVar:     false,
				isFile:    true,
				isLiteral: false,
			},
			{
				name:      "RAILS_ENV",
				isVar:     false,
				isFile:    false,
				isLiteral: true,
			},
			{
				name:      "SOME_ESCAPING_VAR",
				path:      "FOO$BAR",
				isVar:     false,
				isFile:    false,
				isLiteral: true,
			},
			{
				name:         "DEFAULT_VAR",
				path:         "prod/sentry/api_key",
				defaultValue: "defaultvalue",
				isVar:        true,
				isFile:       true,
				isLiteral:    false,
			},
			{
				name:         "DEFAULT_VAR2",
				path:         "foo",
				defaultValue: "def",
				isVar:        false,
				isFile:       false,
				isLiteral:    true,
			},
			{
				name:      "FLOAT",
				path:      "27.1111",
				isVar:     false,
				isFile:    false,
				isLiteral: true,
			},
			{
				name:      "INT",
				path:      "27",
				isVar:     false,
				isFile:    false,
				isLiteral: true,
			},
			{
				name:      "BOOL",
				path:      "true",
				isVar:     false,
				isFile:    false,
				isLiteral: true,
			},
		}
		t.Run("It should correctly identify the types from tags", func(t *testing.T) {
			config, err := ParseFromString(input, testEnv, map[string]string{"env": "prod"})
			assert.NoError(t, err)
			assert.NotNil(t, config)

			validateTestCases(t, testCases, config.EnvSecrets)
		})

		t.Run("With environment in secrets.yml format", func(t *testing.T) {
			testEnv = "TestEnvironment"
			// Use the same input as before, except add the "TestEnvironment" header and indent all the other lines
			input = "TestEnvironment:" + strings.ReplaceAll(input, "\n", "\n  ")

			t.Run("It should correctly identify the types from tags", func(t *testing.T) {
				config, err := ParseFromString(input, testEnv, map[string]string{"env": "prod"})
				assert.NoError(t, err)
				assert.NotNil(t, config)

				validateTestCases(t, testCases, config.EnvSecrets)
			})
		})
	})

	t.Run("Given an empty variable in secrets.yml", func(t *testing.T) {
		testEnv := "TestEnvironment"
		input := `TestEnvironment:
  SOME_VAR1: !var $env/sentry/api_key
  EMPTY_VAR:
  SOME_VAR2: !var:file $env/aws/ec2/private_key`

		t.Run("It should correctly parse the file", func(t *testing.T) {
			config, err := ParseFromString(input, testEnv, map[string]string{"env": "prod"})
			assert.NoError(t, err)

			spec := config.EnvSecrets["EMPTY_VAR"]
			assert.False(t, spec.IsVar())
			assert.False(t, spec.IsFile())
			assert.True(t, spec.IsLiteral())
			assert.Equal(t, "", spec.Path)
		})
	})

	t.Run("Error cases", func(t *testing.T) {
		errorTests := []struct {
			name          string
			input         string
			env           string
			subs          map[string]string
			expectedError string
		}{
			{
				name: "Incorrect/unavailable environment",
				input: `common:
  SOMETHING_COMMON: should-be-available
  RAILS_ENV: should-be-overridden

MissingEnvironment:
  RAILS_ENV: $env`,
				env:           "TestEnvironment",
				subs:          map[string]string{"env": "prod"},
				expectedError: "No such environment 'TestEnvironment' found in secrets file",
			},
			{
				name:          "Environment with no sections",
				input:         `SOME_VAR: value`,
				env:           "TestEnvironment",
				subs:          map[string]string{"env": "prod"},
				expectedError: "No such environment 'TestEnvironment' found in secrets file",
			},
		}

		for _, tt := range errorTests {
			t.Run(tt.name, func(t *testing.T) {
				_, err := ParseFromString(tt.input, tt.env, tt.subs)
				assert.Error(t, err)
				assert.EqualError(t, err, tt.expectedError)
			})
		}
	})

	t.Run("Common section merging", func(t *testing.T) {
		mergeTests := []struct {
			name        string
			sectionName string
			description string
		}{
			{
				name:        "common section",
				sectionName: "common",
				description: "It should merge the environment section with common section",
			},
			{
				name:        "default section",
				sectionName: "default",
				description: "It should merge the environment section with default section",
			},
		}

		for _, tt := range mergeTests {
			t.Run(tt.name, func(t *testing.T) {
				testEnv := "TestEnvironment"
				input := fmt.Sprintf(`%s:
  SOMETHING_COMMON: should-be-available
  RAILS_ENV: should-be-overridden

TestEnvironment:
  RAILS_ENV: $env`, tt.sectionName)

				config, err := ParseFromString(input, testEnv, map[string]string{"env": "prod"})
				assert.NoError(t, err)

				spec := config.EnvSecrets["SOMETHING_COMMON"]
				assert.True(t, spec.IsLiteral())
				assert.Equal(t, "should-be-available", spec.Path)

				// RAILS_ENV should be overridden (specific section takes precedence)
				spec = config.EnvSecrets["RAILS_ENV"]
				assert.True(t, spec.IsLiteral())
				assert.Equal(t, "prod", spec.Path)
			})
		}
	})
}

func validateTestCases(t *testing.T, testCases []testCase, parsed SecretsMap) {
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			spec, found := parsed[tc.name]
			assert.True(t, found)
			if tc.path != "" {
				assert.Equal(t, tc.path, spec.Path)
			}
			if tc.defaultValue != "" {
				assert.Equal(t, tc.defaultValue, spec.DefaultValue)
			}
			assert.Equal(t, tc.isVar, spec.IsVar())
			assert.Equal(t, tc.isFile, spec.IsFile())
			assert.Equal(t, tc.isLiteral, spec.IsLiteral())
		})
	}
}

// Helper function to assert file secrets can be converted to SecretsMap and return it
func assertFileSecrets(t *testing.T, fileConfig FileConfig) SecretsMap {
	t.Helper()
	fileSecrets, ok := fileConfig.Secrets.(SecretsMap)
	if !assert.True(t, ok, "expected file secrets to be SecretsMap") {
		t.FailNow()
	}
	return fileSecrets
}

func TestParseFromString_SimpleFileConfig(t *testing.T) {
	input := `
# Environment variable secrets (backward compatible)
ENV_VARNAME: !var my/conjur/variable

# File-based secrets
summon.files:
  - path: "/path/to/file"
    format: "template"
    template: |
      # This is a file template
      {{ .ENV_VARNAME }}: {{ .ANOTHER_VAR }}
    secrets:
      ENV_VARNAME: !var my/conjur/variable
      ANOTHER_VAR: !var my/other/conjur/variable
`

	config, err := ParseFromString(input, "", nil)
	assert.NoError(t, err)
	assert.NotNil(t, config)

	// Check environment variable secrets
	assert.Len(t, config.EnvSecrets, 1)
	assert.Contains(t, config.EnvSecrets, "ENV_VARNAME")

	// Check file configs
	assert.Len(t, config.Files, 1)
	assert.Equal(t, "/path/to/file", config.Files[0].Path)
	assert.Equal(t, "template", config.Files[0].Format)
	assert.NotEmpty(t, config.Files[0].Template)

	// Check file secrets
	fileSecrets := assertFileSecrets(t, config.Files[0])
	assert.Len(t, fileSecrets, 2)
	assert.Contains(t, fileSecrets, "ENV_VARNAME")
	assert.Contains(t, fileSecrets, "ANOTHER_VAR")
}

func TestParseFromString_MultipleFiles(t *testing.T) {
	input := `
summon.files:
  - path: "/path/to/file1"
    format: "yaml"
    secrets:
      VAR1: !var my/var1

  - path: "/path/to/file2.env"
    format: "dotenv"
    overwrite: true
    permissions: 0644
    secrets:
      VAR2: !var my/var2
`

	config, err := ParseFromString(input, "", nil)
	assert.NoError(t, err)
	assert.NotNil(t, config)

	assert.Len(t, config.Files, 2)

	// Check first file
	assert.Equal(t, "/path/to/file1", config.Files[0].Path)
	assert.Equal(t, "yaml", config.Files[0].Format)
	assert.False(t, config.Files[0].Overwrite)
	assert.Equal(t, os.FileMode(0600), config.Files[0].Permissions)

	// Check second file
	assert.Equal(t, "/path/to/file2.env", config.Files[1].Path)
	assert.Equal(t, "dotenv", config.Files[1].Format)
	assert.True(t, config.Files[1].Overwrite)
	assert.Equal(t, os.FileMode(0644), config.Files[1].Permissions)
}

func TestParseFromString_WithEnvironments(t *testing.T) {
	input := `
# Environment sections for env vars
common:
  DB_HOST: !var common/db/host

dev:
  DB_PASS: !var dev/db/pass

prod:
  DB_PASS: !var prod/db/pass

# File with environment sections
summon.files:
  - path: "/config/app.env"
    format: "dotenv"
    secrets:
      dev:
        API_KEY: !var dev/api/key
      prod:
        API_KEY: !var prod/api/key
`

	// Test with dev environment
	config, err := ParseFromString(input, "dev", nil)
	assert.NoError(t, err)
	assert.NotNil(t, config)

	// Check environment variable secrets include both common and dev
	assert.Len(t, config.EnvSecrets, 2)
	assert.Contains(t, config.EnvSecrets, "DB_HOST")
	assert.Contains(t, config.EnvSecrets, "DB_PASS")

	// Check file secrets are from dev environment
	assert.Len(t, config.Files, 1)
	fileSecrets := assertFileSecrets(t, config.Files[0])
	assert.Contains(t, fileSecrets, "API_KEY")
	assert.Equal(t, "dev/api/key", fileSecrets["API_KEY"].Path)

	// Test with prod environment
	config, err = ParseFromString(input, "prod", nil)
	assert.NoError(t, err)
	assert.NotNil(t, config)

	// Check file secrets are from prod environment
	fileSecrets = assertFileSecrets(t, config.Files[0])
	assert.Contains(t, fileSecrets, "API_KEY")
	assert.Equal(t, "prod/api/key", fileSecrets["API_KEY"].Path)
}

func TestParseFromString_WithSubstitutions(t *testing.T) {
	input := `
summon.files:
  - path: "/path/to/file"
    format: "yaml"
    secrets:
      VAR: !var $env/my/variable
`

	subs := map[string]string{
		"env": "production",
	}

	config, err := ParseFromString(input, "", subs)
	assert.NoError(t, err)
	assert.NotNil(t, config)

	assert.Len(t, config.Files, 1)
	// Verify secrets substitution worked
	fileSecrets := assertFileSecrets(t, config.Files[0])
	assert.Equal(t, "production/my/variable", fileSecrets["VAR"].Path)
}

func TestParseFromString_BackwardCompatible(t *testing.T) {
	// Test that old format with environment sections still works (no summon.files section)
	t.Run("With environment sections", func(t *testing.T) {
		input := `
common:
  ENV_VAR1: !var common/var1
  ENV_VAR2: !var common/var2

custom_env:
  ENV_VAR3: !var custom/var3
`

		config, err := ParseFromString(input, "custom_env", nil)
		assert.NoError(t, err)
		assert.NotNil(t, config)

		// Should have no file configs
		assert.Len(t, config.Files, 0)

		// Should have environment variable secrets (common + custom_env)
		assert.Len(t, config.EnvSecrets, 3)
		assert.Contains(t, config.EnvSecrets, "ENV_VAR1")
		assert.Contains(t, config.EnvSecrets, "ENV_VAR2")
		assert.Contains(t, config.EnvSecrets, "ENV_VAR3")
	})
}

func TestParseFromString_OnlyFiles(t *testing.T) {
	// Test configuration with only file-based secrets
	input := `
summon.files:
  - path: "/path/to/file"
    format: "yaml"
    secrets:
      VAR: !var my/variable
`

	config, err := ParseFromString(input, "", nil)
	assert.NoError(t, err)
	assert.NotNil(t, config)

	// Should have file configs
	assert.Len(t, config.Files, 1)

	// Should have no environment variable secrets
	assert.Len(t, config.EnvSecrets, 0)
}

func TestParseFromString_DefaultPermissions(t *testing.T) {
	input := `
summon.files:
  - path: "/path/to/file"
    secrets:
      VAR: !var my/variable
`

	config, err := ParseFromString(input, "", nil)
	assert.NoError(t, err)
	assert.NotNil(t, config)

	// Should use default permissions (0600)
	assert.Equal(t, os.FileMode(0600), config.Files[0].Permissions)

	// Should use default format (template)
	assert.Equal(t, "template", config.Files[0].Format)
}

func TestParseFromString_FileConfigErrors(t *testing.T) {
	errorTests := []struct {
		name          string
		input         string
		env           string
		expectedError string
	}{
		{
			name: "Missing secrets",
			input: `
summon.files:
  - path: "/path/to/file"
    format: "yaml"
`,
			env:           "",
			expectedError: "no secrets defined",
		},
		{
			name: "Invalid environment",
			input: `
summon.files:
  - path: "/path/to/file"
    secrets:
      dev:
        VAR: !var dev/variable
      prod:
        VAR: !var prod/variable
`,
			env:           "staging",
			expectedError: "No such environment 'staging' found in file config",
		},
	}

	for _, tt := range errorTests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseFromString(tt.input, tt.env, nil)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.expectedError)
		})
	}
}

func TestParseFromFile(t *testing.T) {
	// Create a temporary file with test configuration
	tmpDir := t.TempDir()
	tmpfile := tmpDir + "/secrets.yml"

	content := `
ENV_VAR: !var my/var
summon.files:
  - path: "/path/to/file"
    format: "yaml"
    secrets:
      FILE_VAR: !var my/file/var
`
	err := os.WriteFile(tmpfile, []byte(content), 0644)
	assert.NoError(t, err)

	// Test parsing from file
	config, err := ParseFromFile(tmpfile, "", nil)
	assert.NoError(t, err)
	assert.NotNil(t, config)

	// Verify both env vars and files were parsed
	assert.Len(t, config.EnvSecrets, 1)
	assert.Contains(t, config.EnvSecrets, "ENV_VAR")
	assert.Len(t, config.Files, 1)
	assert.Equal(t, "/path/to/file", config.Files[0].Path)

	// Test with non-existent file
	_, err = ParseFromFile("/nonexistent/file.yml", "", nil)
	assert.Error(t, err)
}
