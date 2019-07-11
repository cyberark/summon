package secretsyml

import (
	"fmt"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestParseFromString(t *testing.T) {
	Convey("Given a string in secrets.yml format", t, func() {
		testEnv := ""
		input := `
SENTRY_API_KEY: !var $env/sentry/api_key
PRIVATE_KEY_FILE: !file:var $env/aws/ec2/private_key
PRIVATE_KEY_FILE2: !var:file $env/aws/ec2/private_key
SOME_FILE: !file my content
RAILS_ENV: $env
FLOAT: 27.1111
INT: 27
BOOL: true`
		Convey("It should correctly identify the types from tags", func() {
			parsed, err := ParseFromString(input, testEnv, map[string]string{"env": "prod"})
			So(err, ShouldBeNil)

			spec := parsed["SENTRY_API_KEY"]
			So(spec.IsVar(), ShouldBeTrue)
			So(spec.IsFile(), ShouldBeFalse)
			So(spec.IsLiteral(), ShouldBeFalse)

			// order of tag declaration shouldn't matter
			for _, key := range []string{"PRIVATE_KEY_FILE", "PRIVATE_KEY_FILE2"} {
				spec = parsed[key]
				So(spec.IsVar(), ShouldBeTrue)
				So(spec.IsFile(), ShouldBeTrue)
				So(spec.IsLiteral(), ShouldBeFalse)
			}

			spec = parsed["SOME_FILE"]
			So(spec.IsVar(), ShouldBeFalse)
			So(spec.IsFile(), ShouldBeTrue)
			So(spec.IsLiteral(), ShouldBeFalse)

			spec = parsed["RAILS_ENV"]
			So(spec.IsVar(), ShouldBeFalse)
			So(spec.IsFile(), ShouldBeFalse)
			So(spec.IsLiteral(), ShouldBeTrue)

			spec, found := parsed["FLOAT"]
			So(found, ShouldBeTrue)
			So(spec.IsLiteral(), ShouldBeTrue)
			So(spec.Path, ShouldEqual, "27.1111")

			spec, found = parsed["INT"]
			So(found, ShouldBeTrue)
			So(spec.IsLiteral(), ShouldBeTrue)
			So(spec.Path, ShouldEqual, "27")

			spec, found = parsed["BOOL"]
			So(found, ShouldBeTrue)
			So(spec.IsLiteral(), ShouldBeTrue)
			So(spec.Path, ShouldEqual, "true")
		})
	})

	Convey("Given a string with environment in secrets.yml format", t, func() {
		testEnv := "TestEnvironment"
		input := `TestEnvironment:
  SENTRY_API_KEY: !var $env/sentry/api_key
  PRIVATE_KEY_FILE: !file:var $env/aws/ec2/private_key
  PRIVATE_KEY_FILE2: !var:file $env/aws/ec2/private_key
  SOME_FILE: !file my content
  RAILS_ENV: $env
  FLOAT: 27.1111
  INT: 27
  BOOL: true`

		Convey("It should correctly identify the types from tags", func() {
			parsed, err := ParseFromString(input, testEnv, map[string]string{"env": "prod"})
			So(err, ShouldBeNil)

			spec := parsed["SENTRY_API_KEY"]
			So(spec.IsVar(), ShouldBeTrue)
			So(spec.IsFile(), ShouldBeFalse)
			So(spec.IsLiteral(), ShouldBeFalse)

			// order of tag declaration shouldn't matter
			for _, key := range []string{"PRIVATE_KEY_FILE", "PRIVATE_KEY_FILE2"} {
				spec = parsed[key]
				So(spec.IsVar(), ShouldBeTrue)
				So(spec.IsFile(), ShouldBeTrue)
				So(spec.IsLiteral(), ShouldBeFalse)
			}

			spec = parsed["SOME_FILE"]
			So(spec.IsVar(), ShouldBeFalse)
			So(spec.IsFile(), ShouldBeTrue)
			So(spec.IsLiteral(), ShouldBeFalse)

			spec = parsed["RAILS_ENV"]
			So(spec.IsVar(), ShouldBeFalse)
			So(spec.IsFile(), ShouldBeFalse)
			So(spec.IsLiteral(), ShouldBeTrue)

			spec, found := parsed["FLOAT"]
			So(found, ShouldBeTrue)
			So(spec.IsLiteral(), ShouldBeTrue)
			So(spec.Path, ShouldEqual, "27.1111")

			spec, found = parsed["INT"]
			So(found, ShouldBeTrue)
			So(spec.IsLiteral(), ShouldBeTrue)
			So(spec.Path, ShouldEqual, "27")

			spec, found = parsed["BOOL"]
			So(found, ShouldBeTrue)
			So(spec.IsLiteral(), ShouldBeTrue)
			So(spec.Path, ShouldEqual, "true")
		})
	})

	Convey("Given an incorrect/unavailable environment", t, func() {
		testEnv := "TestEnvironment"
		input := `common:
  SOMETHING_COMMON: should-be-available
  RAILS_ENV: should-be-overridden

MissingEnvironment:
  RAILS_ENV: $env`
		Convey("It should error", func() {
			_, err := ParseFromString(input, testEnv, map[string]string{"env": "prod"})
			So(err, ShouldNotBeNil)

			errMessage := fmt.Sprintf("No such environment '%v' found in secrets file", testEnv)
			So(err.Error(), ShouldEqual, errMessage)
		})
	})

	Convey("Given a common section and environment ", t, func() {
		testEnv := "TestEnvironment"
		input := `common:
  SOMETHING_COMMON: should-be-available
  RAILS_ENV: should-be-overridden

TestEnvironment:
  RAILS_ENV: $env`

		Convey("It should merge the environment section with common section", func() {
			parsed, err := ParseFromString(input, testEnv, map[string]string{"env": "prod"})
			So(err, ShouldBeNil)

			spec := parsed["SOMETHING_COMMON"]
			So(spec.IsLiteral(), ShouldBeTrue)
			So(spec.Path, ShouldEqual, "should-be-available")

			// RAILS_ENV should be overridden (specific section takes precedence)
			spec = parsed["RAILS_ENV"]
			So(spec.IsLiteral(), ShouldBeTrue)
			So(spec.Path, ShouldEqual, "prod")
		})
	})

	// Verify that 'default' works in addition to 'common'
	Convey("Given a default section and environment ", t, func() {
		testEnv := "TestEnvironment"
		input := `default:
  SOMETHING_COMMON: should-be-available
  RAILS_ENV: should-be-overridden

TestEnvironment:
  RAILS_ENV: $env`

		Convey("It should merge the environment section with default section", func() {
			parsed, err := ParseFromString(input, testEnv, map[string]string{"env": "prod"})
			So(err, ShouldBeNil)

			spec := parsed["SOMETHING_COMMON"]
			So(spec.IsLiteral(), ShouldBeTrue)
			So(spec.Path, ShouldEqual, "should-be-available")

			// RAILS_ENV should be overridden (specific section takes precedence)
			spec = parsed["RAILS_ENV"]
			So(spec.IsLiteral(), ShouldBeTrue)
			So(spec.Path, ShouldEqual, "prod")
		})
	})

}
