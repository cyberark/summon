package secretsyml

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseFromString(t *testing.T) {
	t.Run("Given a string in secrets.yml format", func(t *testing.T) {
		testEnv := ""
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
		t.Run("It should correctly identify the types from tags", func(t *testing.T) {
			parsed, err := ParseFromString(input, testEnv, map[string]string{"env": "prod"})
			assert.NoError(t, err)

			spec := parsed["SENTRY_API_KEY"]
			assert.True(t, spec.IsVar())
			assert.False(t, spec.IsFile())
			assert.False(t, spec.IsLiteral())

			// order of tag declaration shouldn't matter
			for _, key := range []string{"PRIVATE_KEY_FILE", "PRIVATE_KEY_FILE2"} {
				spec = parsed[key]
				assert.True(t, spec.IsVar())
				assert.True(t, spec.IsFile())
				assert.False(t, spec.IsLiteral())
			}

			spec = parsed["SOME_FILE"]
			assert.False(t, spec.IsVar())
			assert.True(t, spec.IsFile())
			assert.False(t, spec.IsLiteral())

			spec = parsed["RAILS_ENV"]
			assert.False(t, spec.IsVar())
			assert.False(t, spec.IsFile())
			assert.True(t, spec.IsLiteral())

			spec = parsed["SOME_ESCAPING_VAR"]
			assert.False(t, spec.IsVar())
			assert.False(t, spec.IsFile())
			assert.True(t, spec.IsLiteral())
			assert.Equal(t, "FOO$BAR", spec.Path)

			spec = parsed["DEFAULT_VAR"]
			assert.True(t, spec.IsFile())
			assert.True(t, spec.IsVar())
			assert.False(t, spec.IsLiteral())
			assert.Equal(t, "prod/sentry/api_key", spec.Path)
			assert.Equal(t, "defaultvalue", spec.DefaultValue)

			spec = parsed["DEFAULT_VAR2"]
			assert.False(t, spec.IsFile())
			assert.False(t, spec.IsVar())
			assert.True(t, spec.IsLiteral())
			assert.Equal(t, "foo", spec.Path)
			assert.Equal(t, "def", spec.DefaultValue)

			spec, found := parsed["FLOAT"]
			assert.True(t, found)
			assert.False(t, spec.IsFile())
			assert.False(t, spec.IsVar())
			assert.True(t, spec.IsLiteral())
			assert.Equal(t, "27.1111", spec.Path)

			spec, found = parsed["INT"]
			assert.True(t, found)
			assert.True(t, spec.IsLiteral())
			assert.Equal(t, "27", spec.Path)

			spec, found = parsed["BOOL"]
			assert.True(t, found)
			assert.True(t, spec.IsLiteral())
			assert.Equal(t, "true", spec.Path)
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

	t.Run("Given a string with environment in secrets.yml format", func(t *testing.T) {
		testEnv := "TestEnvironment"
		input := `TestEnvironment:
  SENTRY_API_KEY: !var $env/sentry/api_key
  PRIVATE_KEY_FILE: !file:var $env/aws/ec2/private_key
  PRIVATE_KEY_FILE2: !var:file $env/aws/ec2/private_key
  SOME_FILE: !file my content
  DEFAULT_VAR: !default='def' $env/value
  RAILS_ENV: $env
  FLOAT: 27.1111
  INT: 27
  BOOL: true`

		t.Run("It should correctly identify the types from tags", func(t *testing.T) {
			parsed, err := ParseFromString(input, testEnv, map[string]string{"env": "prod"})
			assert.NoError(t, err)

			spec := parsed["SENTRY_API_KEY"]
			assert.True(t, spec.IsVar())
			assert.False(t, spec.IsFile())
			assert.False(t, spec.IsLiteral())

			// order of tag declaration shouldn't matter
			for _, key := range []string{"PRIVATE_KEY_FILE", "PRIVATE_KEY_FILE2"} {
				spec = parsed[key]
				assert.True(t, spec.IsVar())
				assert.True(t, spec.IsFile())
				assert.False(t, spec.IsLiteral())
			}

			spec = parsed["SOME_FILE"]
			assert.False(t, spec.IsVar())
			assert.True(t, spec.IsFile())
			assert.False(t, spec.IsLiteral())

			spec = parsed["DEFAULT_VAR"]
			assert.False(t, spec.IsVar())
			assert.False(t, spec.IsFile())
			assert.True(t, spec.IsLiteral())
			assert.Equal(t, "prod/value", spec.Path)

			spec = parsed["RAILS_ENV"]
			assert.False(t, spec.IsVar())
			assert.False(t, spec.IsFile())
			assert.True(t, spec.IsLiteral())

			spec, found := parsed["FLOAT"]
			assert.True(t, found)
			assert.True(t, spec.IsLiteral())
			assert.Equal(t, "27.1111", spec.Path)

			spec, found = parsed["INT"]
			assert.True(t, found)
			assert.True(t, spec.IsLiteral())
			assert.Equal(t, "27", spec.Path)

			spec, found = parsed["BOOL"]
			assert.True(t, found)
			assert.True(t, spec.IsLiteral())
			assert.Equal(t, "true", spec.Path)
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
