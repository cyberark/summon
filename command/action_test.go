package command

import (
	"github.com/conjurinc/cauldron/secretsyml"
	. "github.com/smartystreets/goconvey/convey"
	"io/ioutil"
	"os"
	"path"
	"strings"
	"testing"
)

func TestRunAction(t *testing.T) {
	providerPath := path.Join(os.Getenv("PWD"), "testprovider.sh")

	Convey("Passing in secrets.yml via --yaml", t, func() {
		out, err := runAction(
			[]string{"printenv", "MYVAR"},
			providerPath,
			"",
			"MYVAR: !var somesecret/on/a/path",
			map[string]string{},
			[]string{},
		)
		So(out, ShouldEqual, "mysecret\n")
		So(err, ShouldBeNil)
	})

	Convey("Errors when fetching keys keys return error", t, func() {
		_, err := runAction(
			[]string{"printenv", "MYVAR"},
			providerPath,
			"",
			"MYVAR: !var error",
			map[string]string{},
			[]string{},
		)
		So(err, ShouldNotBeNil)
	})

	Convey("Errors when fetching keys keys don't return error if ignored", t, func() {
		out, err := runAction(
			[]string{"printenv", "MYVAR"},
			providerPath,
			"",
			"{MYVAR: !var test, ERR: !var error}",
			map[string]string{},
			[]string{"ERR"},
		)
		So(err, ShouldBeNil)
		So(out, ShouldEqual, "mysecret\n")
	})
}

func TestConvertSubsToMap(t *testing.T) {
	Convey("Substitutions are returned as a map used later for interpolation", t, func() {
		input := []string{
			"policy=accounts-database",
			"environment=production",
		}

		expected := map[string]string{
			"policy":      "accounts-database",
			"environment": "production",
		}

		output := convertSubsToMap(input)

		So(output, ShouldResemble, expected)
	})
}

func TestRunSubcommand(t *testing.T) {
	Convey("The subcommand should have access to secrets injected into its environment", t, func() {
		args := []string{"printenv", "MYVAR"}
		env := []string{"MYVAR=myvalue"}

		output, err := runSubcommand(args, env)
		expected := "myvalue\n"

		So(output, ShouldEqual, expected)
		So(err, ShouldBeNil)
	})
}

func TestFormatForEnvString(t *testing.T) {
	Convey("formatForEnv should return a KEY=VALUE string that can be appended to an environment", t, func() {
		Convey("For variables, VALUE should be the value of the secret", func() {
			envvar := formatForEnv(
				"DBPASS",
				"mysecretvalue",
				secretsyml.SecretSpec{Path: "mysql1/password", Kind: secretsyml.SecretVar},
				nil,
			)

			So(envvar, ShouldEqual, "DBPASS=mysecretvalue")
		})
		Convey("For files, VALUE should be the path to a tempfile containing the secret", func() {
			temp_factory := NewTempFactory("")
			defer temp_factory.Cleanup()

			envvar := formatForEnv(
				"SSL_CERT",
				"mysecretvalue",
				secretsyml.SecretSpec{Path: "certs/webtier1/private-cert", Kind: secretsyml.SecretFile},
				&temp_factory,
			)

			s := strings.Split(envvar, "=")
			key, path := s[0], s[1]

			So(key, ShouldEqual, "SSL_CERT")

			// Temp path should exist
			_, err := os.Stat(path)
			So(err, ShouldBeNil)

			contents, _ := ioutil.ReadFile(path)

			So(string(contents), ShouldContainSubstring, "mysecretvalue")
		})
	})
}
