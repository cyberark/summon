package secretsyml

import (
	"reflect"
	"testing"
)

func TestParseFromString(t *testing.T) {
	input := `
  SENTRY_API_KEY: !var $env/sentry/api_key
  RAILS_ENV: $env
  PRIVATE_KEY_FILE: !file $env/aws/ec2/private_key
  `
	expected := SecretsMap{
		"SENTRY_API_KEY":   SecretSpec{Path: "prod/sentry/api_key", Kind: SecretVar},
		"PRIVATE_KEY_FILE": SecretSpec{Path: "prod/aws/ec2/private_key", Kind: SecretFile},
		"RAILS_ENV": SecretSpec{Path: "prod", Kind: SecretLiteral},
	}

	yml, err := ParseFromString(input, map[string]string{"$env": "prod"})
	if err != nil {
		t.Error(err)
	}

	if !reflect.DeepEqual(expected, yml) {
		t.Errorf("\nexpected\n%s\ngot\n%s", expected, yml)
	}
}
