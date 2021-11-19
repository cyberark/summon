package secretsyml

import (
	"fmt"
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
			parsed, err := ParseFromString(input, testEnv, map[string]string{"env": "prod"})
			assert.NoError(t, err)
			assert.NotNil(t, parsed)

			validateTestCases(t, testCases, parsed)
		})

		t.Run("With environment in secrets.yml format", func(t *testing.T) {
			testEnv = "TestEnvironment"
			// Use the same input as before, except add the "TestEnvironment" header and indent all the other lines
			input = "TestEnvironment:" + strings.ReplaceAll(input, "\n", "\n  ")

			t.Run("It should correctly identify the types from tags", func(t *testing.T) {
				parsed, err := ParseFromString(input, testEnv, map[string]string{"env": "prod"})
				assert.NoError(t, err)
				assert.NotNil(t, parsed)

				validateTestCases(t, testCases, parsed)
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
			parsed, err := ParseFromString(input, testEnv, map[string]string{"env": "prod"})
			assert.NoError(t, err)

			spec := parsed["EMPTY_VAR"]
			assert.False(t, spec.IsVar())
			assert.False(t, spec.IsFile())
			assert.True(t, spec.IsLiteral())
			assert.Equal(t, "", spec.Path)
		})
	})

	t.Run("Given an incorrect/unavailable environment", func(t *testing.T) {
		testEnv := "TestEnvironment"
		input := `common:
  SOMETHING_COMMON: should-be-available
  RAILS_ENV: should-be-overridden

MissingEnvironment:
  RAILS_ENV: $env`
		t.Run("It should error", func(t *testing.T) {
			_, err := ParseFromString(input, testEnv, map[string]string{"env": "prod"})
			assert.Error(t, err)

			errMessage := fmt.Sprintf("No such environment '%v' found in secrets file", testEnv)
			assert.EqualError(t, err, errMessage)
		})
	})

	t.Run("Given a common section and environment ", func(t *testing.T) {
		testEnv := "TestEnvironment"
		input := `common:
  SOMETHING_COMMON: should-be-available
  RAILS_ENV: should-be-overridden

TestEnvironment:
  RAILS_ENV: $env`

		t.Run("It should merge the environment section with common section", func(t *testing.T) {
			parsed, err := ParseFromString(input, testEnv, map[string]string{"env": "prod"})
			assert.NoError(t, err)

			spec := parsed["SOMETHING_COMMON"]
			assert.True(t, spec.IsLiteral())
			assert.Equal(t, "should-be-available", spec.Path)

			// RAILS_ENV should be overridden (specific section takes precedence)
			spec = parsed["RAILS_ENV"]
			assert.True(t, spec.IsLiteral())
			assert.Equal(t, "prod", spec.Path)
		})
	})

	// Verify that 'default' works in addition to 'common'
	t.Run("Given a default section and environment ", func(t *testing.T) {
		testEnv := "TestEnvironment"
		input := `default:
  SOMETHING_COMMON: should-be-available
  RAILS_ENV: should-be-overridden

TestEnvironment:
  RAILS_ENV: $env`

		t.Run("It should merge the environment section with default section", func(t *testing.T) {
			parsed, err := ParseFromString(input, testEnv, map[string]string{"env": "prod"})
			assert.NoError(t, err)

			spec := parsed["SOMETHING_COMMON"]
			assert.True(t, spec.IsLiteral())
			assert.Equal(t, "should-be-available", spec.Path)

			// RAILS_ENV should be overridden (specific section takes precedence)
			spec = parsed["RAILS_ENV"]
			assert.True(t, spec.IsLiteral())
			assert.Equal(t, "prod", spec.Path)
		})
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
