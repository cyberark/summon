package command

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	. "github.com/smartystreets/goconvey/convey"
	_ "golang.org/x/net/context"

	"github.com/cyberark/summon/secretsyml"
)

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

func TestFormatForEnvString(t *testing.T) {
	Convey("formatForEnv should return a KEY=VALUE string that can be appended to an environment", t, func() {
		Convey("For variables, VALUE should be the value of the secret", func() {
			spec := secretsyml.SecretSpec{
				Path: "mysql1/password",
				Tags: []secretsyml.YamlTag{secretsyml.Var},
			}
			k, v := formatForEnv(
				"dbpass",
				"mysecretvalue",
				spec,
				nil,
			)

			So(k, ShouldEqual, "dbpass")
			So(v, ShouldEqual, "mysecretvalue")
		})
		Convey("For files, VALUE should be the path to a tempfile containing the secret", func() {
			tempFactory := NewTempFactory("")
			defer tempFactory.Cleanup()

			spec := secretsyml.SecretSpec{
				Path: "certs/webtier1/private-cert",
				Tags: []secretsyml.YamlTag{secretsyml.File},
			}
			key, path := formatForEnv(
				"SSL_CERT",
				"mysecretvalue",
				spec,
				&tempFactory,
			)

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
		result := joinEnv(map[string]string{"foo": "bar", "baz": "qux"})
		So(result, ShouldEqual, "baz=qux\nfoo=bar\n")
	})
}

func TestRunAction(t *testing.T) {
	Convey("Variable resolution correctly resolves variables", t, func() {
		expectedValue := "valueOfVariable"

		dir, err := ioutil.TempDir("", "summon")
		So(err, ShouldBeNil)
		if err != nil {
			return
		}
		defer os.RemoveAll(dir)

		tempFile := filepath.Join(dir, "outputFile.txt")

		err = runAction(&ActionConfig{
			Args:       []string{"bash", "-c", "echo -n \"$FOO\" > " + tempFile},
			YamlInline: "FOO: " + expectedValue,
		})

		code, err := returnStatusOfError(err)
		So(err, ShouldBeNil)
		So(code, ShouldEqual, 0)

		if err != nil || code != 0 {
			return
		}

		content, err := ioutil.ReadFile(tempFile)
		So(err, ShouldBeNil)
		if err != nil {
			return
		}

		So(string(content), ShouldEqual, expectedValue)
	})

	Convey("Docker options correctly injected for top-level command", t, func() {
		RunDockerArgsTestCase(t, func (dockerDaemonSocket string) []string {
			return []string{
				"docker",
				"-H", dockerDaemonSocket,
				"run",
				"--rm", "-d", "@SUMMONDOCKERARGS",
				"alpine",
			}
		})
	})

	Convey("Docker options correctly injected for nested command", t, func() {
		RunDockerArgsTestCase(t, func (dockerDaemonSocket string) []string {
			return []string{
				"sh",
				"-c",
				"docker -H "+dockerDaemonSocket+" run --rm -d @SUMMONDOCKERARGS alpine",
			}
		})
	})
}

func RunDockerArgsTestCase(
	t *testing.T,
	dockerCommandGen func(dockerDaemonSocket string,
) []string) {
	// This is a test case for @SUMMONDOCKERARGS. It exercises Docker CLI pointed to a mock
	// server. It asserts on the request payload received on the container creation
	// endpoint, the volume mounts and environment variables injected by summon are
	// expected to be present.

	expected := map[string]string{
		"A": "A's multiple line\nvalue",
		"B": "B_value",
		"C": "C_value",
		"D": "D_value",
	}
	const inlineSecretsYml = `
A: |-
 A's multiple line
 value
B: !var B_value
C: !file C_value
D: !var:file D_value
`

	volumeBinds := map[string]struct{
		ContainerPath string
		FileContents string
	}{}
	envvars := map[string]string{}

	// Mock server for handling API calls by `docker run`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			*container.Config
			HostConfig       *container.HostConfig
		}

		if !regexp.MustCompile("/.*/containers/create").MatchString(r.URL.Path) {
			// Mock response to all the other endpoints called as part of `docker run`
			w.WriteHeader(200)
			fmt.Fprintln(w,  "{}")
			return
		}
		payloadBytes, err := ioutil.ReadAll(r.Body)

		if err != nil {
			t.Errorf("failure reading payload from docker cli: %s", err)
			return
		}
		err = json.Unmarshal(payloadBytes, &payload)
		if err != nil {
			t.Errorf("payload from docker cli could not be parsed: %s", err)
			return
		}

		for _, env := range payload.Env {
			nameAndValue := strings.SplitN(env, "=", 2)
			name := nameAndValue[0]
			value := nameAndValue[1]

			envvars[name] = value
		}

		for _, volumeBind := range payload.HostConfig.Binds {
			fromAndTo := strings.SplitN(volumeBind, ":", 2)
			from := fromAndTo[0]
			to := fromAndTo[1]

			fileContents, _ := ioutil.ReadFile(from)
			volumeBinds[from] = struct {
				ContainerPath string
				FileContents  string
			}{
				ContainerPath: to,
				FileContents: string(fileContents),
			}
		}

		w.WriteHeader(201)

		// Mock response to container create endpoint
		fmt.Fprintln(w,  `{"Id": "e90e34656806", "Warnings": []}`)
	}))
	defer ts.Close()

	var stdBuf bytes.Buffer
	// Run docker wrapped around summon and leveraging @SUMMONDOCKERARGS
	err := runAction(&ActionConfig{
		StdErr: &stdBuf,
		StdOut: &stdBuf,
		Provider: "/bin/echo", // Use /bin/echo provider for brevity
		Args: dockerCommandGen(strings.Replace(ts.URL, "http://", "tcp://", 1)),
		YamlInline: inlineSecretsYml,
	})

	// Make assertions
	code, err := returnStatusOfError(err)
	So(err, ShouldBeNil)
	So(code, ShouldEqual, 0)

	// The volume mount binds are expected to take the form
	// 'host_path:container_path', where host_path is equal to container_path
	for from, volumeBind := range volumeBinds {
		So(from, ShouldEqual, volumeBind.ContainerPath)
	}

	// Ensure envvars and volumemounts passed to Docker match expectations
	So(envvars["A"], ShouldEqual, expected["A"])
	So(envvars["B"], ShouldEqual, expected["B"])
	So(volumeBinds[envvars["C"]].FileContents, ShouldEqual, expected["C"])
	So(volumeBinds[envvars["D"]].FileContents, ShouldEqual, expected["D"])
}

func TestDefaultVariableResolution(t *testing.T) {
	Convey("Variable resolution correctly resolves variables", t, func() {
		expectedDefaultValue := "defaultValueOfVariable"

		dir, err := ioutil.TempDir("", "summon")
		So(err, ShouldBeNil)
		if err != nil {
			return
		}
		defer os.RemoveAll(dir)

		tempFile := filepath.Join(dir, "outputFile.txt")

		err = runAction(&ActionConfig{
			Args:       []string{"bash", "-c", "echo -n \"$FOO\" > " + tempFile},
			YamlInline: "FOO: !str:default='" + expectedDefaultValue + "'",
		})

		code, err := returnStatusOfError(err)
		So(err, ShouldBeNil)
		So(code, ShouldEqual, 0)

		if err != nil || code != 0 {
			return
		}

		content, err := ioutil.ReadFile(tempFile)
		So(err, ShouldBeNil)
		if err != nil {
			return
		}

		So(string(content), ShouldEqual, expectedDefaultValue)
	})
}

func TestDefaultVariableResolutionWithValue(t *testing.T) {
	Convey("Variable resolution correctly resolves variables", t, func() {
		expectedValue := "valueOfVariable"

		dir, err := ioutil.TempDir("", "summon")
		So(err, ShouldBeNil)
		if err != nil {
			return
		}
		defer os.RemoveAll(dir)

		tempFile := filepath.Join(dir, "outputFile.txt")

		err = runAction(&ActionConfig{
			Args:       []string{"bash", "-c", "echo -n \"$FOO\" > " + tempFile},
			YamlInline: "FOO: !str:default='something' " + expectedValue,
		})

		code, err := returnStatusOfError(err)
		So(err, ShouldBeNil)
		So(code, ShouldEqual, 0)

		if err != nil || code != 0 {
			return
		}

		content, err := ioutil.ReadFile(tempFile)
		So(err, ShouldBeNil)
		if err != nil {
			return
		}

		So(string(content), ShouldEqual, expectedValue)
	})
}

func TestReturnStatusOfError(t *testing.T) {
	Convey("returns no error as 0", t, func() {
		res, err := returnStatusOfError(nil)
		So(res, ShouldEqual, 0)
		So(err, ShouldBeNil)
	})

	Convey("returns ExitError as the wrapped exit status", t, func() {
		exit := exec.Command("false").Run()
		res, err := returnStatusOfError(exit)
		So(res, ShouldEqual, 1)
		So(err, ShouldBeNil)
	})

	Convey("returns other errors unchanged", t, func() {
		expected := errors.New("test")
		_, err := returnStatusOfError(expected)
		So(err, ShouldEqual, expected)
	})
}

func TestPrintProviderVersions(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping long-running test.")
	}

	Convey("printProviderVersions should return a string of all of the providers in the DefaultPath", t, func() {
		pathTo, err := os.Getwd()
		So(err, ShouldBeNil)
		pathToTest := filepath.Join(pathTo, "testversions")

		//test1 - regular formating and appending of version # to string
		//test2 - chopping off of trailing newline
		//test3 - failed `--version` call
		output, err := printProviderVersions(pathToTest)
		So(err, ShouldBeNil)

		expected := `testprovider version 1.2.3
testprovider-noversionsupport: unknown version
testprovider-trailingnewline version 3.2.1
`

		So(output, ShouldEqual, expected)
	})
}

func TestLocateFileRecurseUp(t *testing.T) {
	filename := "test.txt"

	Convey("Finds file in current working directory", t, func() {
		topDir, err := ioutil.TempDir("", "summon")
		So(err, ShouldBeNil)
		defer os.RemoveAll(topDir)

		localFilePath := filepath.Join(topDir, filename)
		_, err = os.Create(localFilePath)
		So(err, ShouldBeNil)

		gotPath, err := findInParentTree(filename, topDir)
		So(err, ShouldBeNil)

		So(gotPath, ShouldEqual, localFilePath)
	})

	Convey("Finds file in a higher working directory", t, func() {
		topDir, err := ioutil.TempDir("", "summon")
		So(err, ShouldBeNil)
		defer os.RemoveAll(topDir)

		higherFilePath := filepath.Join(topDir, filename)
		_, err = os.Create(higherFilePath)
		So(err, ShouldBeNil)

		// Create a downwards directory hierarchy, starting from topDir
		downDir := filepath.Join(topDir, "dir1", "dir2", "dir3")
		err = os.MkdirAll(downDir, 0700)
		So(err, ShouldBeNil)

		gotPath, err := findInParentTree(filename, downDir)
		So(err, ShouldBeNil)

		So(gotPath, ShouldEqual, higherFilePath)
	})

	Convey("returns a friendly error if file not found", t, func() {
		topDir, err := ioutil.TempDir("", "summon")
		So(err, ShouldBeNil)
		defer os.RemoveAll(topDir)

		// A unlikely to exist file name
		nonExistingFileName := strconv.FormatInt(time.Now().Unix(), 10)
		wantErrMsg := fmt.Sprintf(
			"unable to locate file specified (%s): reached root of file system",
			nonExistingFileName)

		_, err = findInParentTree(nonExistingFileName, topDir)
		So(err.Error(), ShouldEqual, wantErrMsg)
	})

	Convey("returns a friendly error if file is an absolute path", t, func() {
		topDir, err := ioutil.TempDir("", "summon")
		So(err, ShouldBeNil)
		defer os.RemoveAll(topDir)

		absFileName := "/foo/bar/baz"
		wantErrMsg := "file specified (/foo/bar/baz) is an absolute path: will not recurse up"

		_, err = findInParentTree(absFileName, topDir)
		So(err.Error(), ShouldEqual, wantErrMsg)
	})

	Convey("returns a friendly error in unexpected circumstances (100% coverage)", t, func() {
		topDir, err := ioutil.TempDir("", "summon")
		So(err, ShouldBeNil)
		defer os.RemoveAll(topDir)

		fileNameWithNulByte := "pizza\x00margherita"
		wantErrMsg := "unable to locate file specified (pizza\x00margherita): stat"

		_, err = findInParentTree(fileNameWithNulByte, topDir)
		So(err.Error(), ShouldStartWith, wantErrMsg)
	})
}
