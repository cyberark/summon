package cauldron

import (
	"github.com/conjurinc/cauldron/secretsyml"
	"io/ioutil"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestConvertSubsToMap(t *testing.T) {
	input := []string{
		"policy=accounts-database",
		"environment=production",
	}

	expected := map[string]string{
		"policy":      "accounts-database",
		"environment": "production",
	}

	output := convertSubsToMap(input)

	if !reflect.DeepEqual(expected, output) {
		t.Errorf("\nexpected\n%s\ngot\n%s", expected, output)
	}
}

// Test running a subcommand with specified environment
func TestRunSubcommand(t *testing.T) {
	args := []string{"printenv", "MYVAR"}
	env := []string{"MYVAR=myvalue"}

	output := runSubcommand(args, env)
	expected := "myvalue\n"

	if output != expected {
		t.Errorf("\nexpected\n%s\ngot\n%s", expected, output)
	}
}

// Test exporting a secret value to env
func TestFetchToEnvironString(t *testing.T) {
	provider := "./provider/provider"

	envvar, err := fetchToEnviron(
		"DBPASS",
		secretsyml.SecretSpec{Path: "mysql1/password", IsFile: false},
		provider,
	)
	if err != nil {
		t.Error(err.Error())
	}

	expected := "DBPASS=mysecretvalue"

	if envvar != expected {
		t.Errorf("\nexpected\n%s\ngot\n%s", expected, envvar)
	}
}

// Test writing value to a tempfile and exporting the path
func TestFetchToEnvironFile(t *testing.T) {
	provider := "./provider/provider"

	envvar, err := fetchToEnviron(
		"SSL_CERT",
		secretsyml.SecretSpec{Path: "certs/webtier1/private-cert", IsFile: true},
		provider,
	)
	if err != nil {
		t.Error(err.Error())
	}

	s := strings.Split(envvar, "=")
	key, path := s[0], s[1]

	expectedKey := "SSL_CERT"
	if key != expectedKey {
		t.Errorf("\nKey:\nexpected\n%s\ngot\n%s", expectedKey, key)
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Errorf("path doesn't exist: %s", path)
	}

	contents, err := ioutil.ReadFile(path)
	if err != nil {
		t.Error(err.Error)
	}

	if string(contents) != "mysecretvalue" {
		t.Errorf("\nFile:\nexpected\n%s\ngot\n%s", "mysecretvalue", string(contents))
	}
}
