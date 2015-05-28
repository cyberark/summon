package secretsyml

import (
	. "github.com/smartystreets/goconvey/convey"
	"testing"
)

func TestParseFromString(t *testing.T) {
	Convey("Given a string in secrets.yml format", t, func() {
		input := `
    SENTRY_API_KEY: !var $env/sentry/api_key
    PRIVATE_KEY_FILE: !file:var $env/aws/ec2/private_key
    PRIVATE_KEY_FILE2: !var:file $env/aws/ec2/private_key
    SOME_FILE: !file my content
    RAILS_ENV: $env
    IGNORED: !float 27.1111
    `
		Convey("It should correctly identify the types from tags", func() {
			parsed, err := ParseFromString(input, map[string]string{"$env": "prod"})
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

			_, found := parsed["IGNORED"]
			So(found, ShouldBeFalse)
		})
	})
}
