package secretsyml

import (
	"reflect"
	"testing"
)

func TestParseFromString(t *testing.T) {
	input := `
  SENTRY_API_KEY: $env/sentry/api_key
  PRIVATE_KEY_FILE: !file $env/aws/ec2/private_key
  `
	expected := SecretsMap{
		"SENTRY_API_KEY":   SecretSpec{Path: "prod/sentry/api_key", IsFile: false},
		"PRIVATE_KEY_FILE": SecretSpec{Path: "prod/aws/ec2/private_key", IsFile: true},
	}

	yml, err := ParseFromString(input, map[string]string{"$env": "prod"})
	if err != nil {
		t.Error(err)
	}

	if !reflect.DeepEqual(expected, yml) {
		t.Errorf("\nexpected\n%s\ngot\n%s", expected, yml)
	}
}
