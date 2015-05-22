package secretsyml

import (
	. "github.com/smartystreets/goconvey/convey"
	"testing"
)

func TestParseFromString(t *testing.T) {
	Convey("Given a string in secrets.yml format", t, func() {
		input := `
    SENTRY_API_KEY: !var $env/sentry/api_key
    RAILS_ENV: $env
    PRIVATE_KEY_FILE: !file $env/aws/ec2/private_key
    IGNORED: !float 27.1111
    `
		Convey("It should parse to a map of env var names to SecretSpecs", func() {
			expected := SecretsMap{
				"SENTRY_API_KEY":   SecretSpec{Path: "prod/sentry/api_key", Kind: SecretVar},
				"PRIVATE_KEY_FILE": SecretSpec{Path: "prod/aws/ec2/private_key", Kind: SecretFile},
				"RAILS_ENV":        SecretSpec{Path: "prod", Kind: SecretLiteral},
			}
			parsed, err := ParseFromString(input, map[string]string{"$env": "prod"})
			So(parsed, ShouldResemble, expected)
			So(err, ShouldBeNil)

			spec := parsed["SENTRY_API_KEY"]
			So(spec.IsVar(), ShouldBeTrue)
			spec = parsed["PRIVATE_KEY_FILE"]
			So(spec.IsFile(), ShouldBeTrue)
			spec = parsed["RAILS_ENV"]
			So(spec.IsLiteral(), ShouldBeTrue)
		})
	})
}
