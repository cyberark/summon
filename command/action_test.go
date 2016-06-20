package command

import (
	"github.com/conjurinc/summon/secretsyml"
	. "github.com/smartystreets/goconvey/convey"
	. "golang.org/x/net/context"
	"io/ioutil"
	"os"
	"path"
	"strings"
	"testing"
)

func TestRunAction(t *testing.T) {
	Convey("Using a dummy provider that returns 'mysecret'", t, func() {
		providerPath := path.Join(os.Getenv("PWD"), "testprovider.sh")

		Convey("Passing in secrets.yml via --yaml", func() {
			out, err := runAction(&ActionConfig{
				Args:       []string{"printenv", "MYVAR"},
				Provider:   providerPath,
				Filepath:   "",
				YamlInline: "MYVAR: !var somesecret/on/a/path",
				Subs:       map[string]string{},
				Ignores:    []string{},
			})
			So(out, ShouldEqual, "mysecret\n")
			So(err, ShouldBeNil)
		})

		Convey("Errors when fetching keys return error", func() {
			_, err := runAction(&ActionConfig{
				Args:       []string{"printenv", "MYVAR"}, // args
				Provider:   providerPath,                  // provider
				Filepath:   "",                            // filepath
				YamlInline: "MYVAR: !var error",           // yaml inline
				Subs:       map[string]string{},           // subs
				Ignores:    []string{},                    // ignore
			})
			So(err, ShouldNotBeNil)
		})

		Convey("Errors when fetching keys don't return error if ignored", func() {
			out, err := runAction(&ActionConfig{
				Args:       []string{"printenv", "MYVAR"},
				Provider:   providerPath,
				Filepath:   "",
				YamlInline: "{MYVAR: !var test, ERR: !var error}",
				Subs:       map[string]string{},
				Ignores:    []string{"ERR"},
			})
			So(err, ShouldBeNil)
			So(out, ShouldEqual, "mysecret\n")
		})
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
			spec := secretsyml.SecretSpec{
				Path: "mysql1/password",
				Tags: []secretsyml.YamlTag{secretsyml.Var},
			}
			envvar := formatForEnv(
				"dbpass",
				"mysecretvalue",
				spec,
				nil,
			)

			So(envvar, ShouldEqual, "dbpass=mysecretvalue")
		})
		Convey("For files, VALUE should be the path to a tempfile containing the secret", func() {
			temp_factory := NewTempFactory("")
			defer temp_factory.Cleanup()

			spec := secretsyml.SecretSpec{
				Path: "certs/webtier1/private-cert",
				Tags: []secretsyml.YamlTag{secretsyml.File},
			}
			envvar := formatForEnv(
				"SSL_CERT",
				"mysecretvalue",
				spec,
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

func TestJoinEnv(t *testing.T) {
	Convey("adds a trailing newline", t, func() {
		result := joinEnv([]string{"foo", "bar"})
		So(result, ShouldEqual, "foo\nbar\n")
	})
}
