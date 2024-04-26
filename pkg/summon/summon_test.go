package summon

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/cyberark/summon/pkg/secretsyml"
	"github.com/stretchr/testify/assert"
)

func TestRunSubprocess(t *testing.T) {
	t.Run("Variable resolution correctly resolves variables", func(t *testing.T) {
		expectedValue := "valueOfVariable"

		dir, err := ioutil.TempDir("", "summon")
		assert.NoError(t, err)
		if err != nil {
			return
		}
		defer os.RemoveAll(dir)

		tempFile := filepath.Join(dir, "outputFile.txt")

		code, err := RunSubprocess(&SubprocessConfig{
			Args:       []string{"bash", "-c", "echo -n \"$FOO\" > " + tempFile},
			YamlInline: "FOO: " + expectedValue,
		})

		assert.NoError(t, err)
		assert.Equal(t, 0, code)

		if err != nil || code != 0 {
			return
		}

		content, err := ioutil.ReadFile(tempFile)
		assert.NoError(t, err)
		if err != nil {
			return
		}

		assert.Equal(t, expectedValue, string(content))
	})
}

func TestDefaultVariableResolution(t *testing.T) {
	t.Run("Variable resolution correctly resolves variables", func(t *testing.T) {
		expectedDefaultValue := "defaultValueOfVariable"

		dir, err := ioutil.TempDir("", "summon")
		assert.NoError(t, err)
		if err != nil {
			return
		}
		defer os.RemoveAll(dir)

		tempFile := filepath.Join(dir, "outputFile.txt")

		code, err := RunSubprocess(&SubprocessConfig{
			Args:       []string{"bash", "-c", "echo -n \"$FOO\" > " + tempFile},
			YamlInline: "FOO: !str:default='" + expectedDefaultValue + "'",
		})

		assert.NoError(t, err)
		assert.Equal(t, 0, code)

		if err != nil || code != 0 {
			return
		}

		content, err := ioutil.ReadFile(tempFile)
		assert.NoError(t, err)
		if err != nil {
			return
		}

		assert.Equal(t, expectedDefaultValue, string(content))
	})
}

func TestDefaultVariableResolutionWithValue(t *testing.T) {
	t.Run("Variable resolution correctly resolves variables", func(t *testing.T) {
		expectedValue := "valueOfVariable"

		dir, err := ioutil.TempDir("", "summon")
		assert.NoError(t, err)
		if err != nil {
			return
		}
		defer os.RemoveAll(dir)

		tempFile := filepath.Join(dir, "outputFile.txt")

		code, err := RunSubprocess(&SubprocessConfig{
			Args:       []string{"bash", "-c", "echo -n \"$FOO\" > " + tempFile},
			YamlInline: "FOO: !str:default='something' " + expectedValue,
		})

		assert.NoError(t, err)
		assert.Equal(t, 0, code)

		if err != nil || code != 0 {
			return
		}

		content, err := ioutil.ReadFile(tempFile)
		assert.NoError(t, err)
		if err != nil {
			return
		}

		assert.Equal(t, expectedValue, string(content))
	})
}
func TestConvertSubsToMap(t *testing.T) {
	t.Run("Substitutions are returned as a map used later for interpolation", func(t *testing.T) {
		input := []string{
			"policy=accounts-database",
			"environment=production",
		}

		expected := map[string]string{
			"policy":      "accounts-database",
			"environment": "production",
		}

		output := convertSubsToMap(input)

		assert.EqualValues(t, expected, output)
	})
}

func TestFormatForEnvString(t *testing.T) {
	t.Run("formatForEnv should return a KEY=VALUE string that can be appended to an environment", func(t *testing.T) {
		t.Run("For variables, VALUE should be the value of the secret", func(t *testing.T) {
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

			assert.Equal(t, "dbpass", k)
			assert.Equal(t, "mysecretvalue", v)
		})
		t.Run("For files, VALUE should be the path to a tempfile containing the secret", func(t *testing.T) {
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

			assert.Equal(t, "SSL_CERT", key)

			// Temp path should exist
			_, err := os.Stat(path)
			assert.NoError(t, err)

			contents, _ := ioutil.ReadFile(path)

			assert.Contains(t, string(contents), "mysecretvalue")
		})
	})
}

func TestJoinEnv(t *testing.T) {
	t.Run("adds a trailing newline", func(t *testing.T) {
		result := joinEnv(map[string]string{"foo": "bar", "baz": "qux"})
		assert.Equal(t, "baz=qux\nfoo=bar\n", result)
	})
}

func TestLocateFileRecurseUp(t *testing.T) {
	filename := "test.txt"

	t.Run("Finds file in current working directory", func(t *testing.T) {
		topDir, err := ioutil.TempDir("", "summon")
		assert.NoError(t, err)
		defer os.RemoveAll(topDir)

		localFilePath := filepath.Join(topDir, filename)
		_, err = os.Create(localFilePath)
		assert.NoError(t, err)

		gotPath, err := findInParentTree(filename, topDir)
		assert.NoError(t, err)

		assert.Equal(t, localFilePath, gotPath)
	})

	t.Run("Finds file in a higher working directory", func(t *testing.T) {
		topDir, err := ioutil.TempDir("", "summon")
		assert.NoError(t, err)
		defer os.RemoveAll(topDir)

		higherFilePath := filepath.Join(topDir, filename)
		_, err = os.Create(higherFilePath)
		assert.NoError(t, err)

		// Create a downwards directory hierarchy, starting from topDir
		downDir := filepath.Join(topDir, "dir1", "dir2", "dir3")
		err = os.MkdirAll(downDir, 0700)
		assert.NoError(t, err)

		gotPath, err := findInParentTree(filename, downDir)
		assert.NoError(t, err)

		assert.Equal(t, higherFilePath, gotPath)
	})

	t.Run("returns a friendly error if file not found", func(t *testing.T) {
		topDir, err := ioutil.TempDir("", "summon")
		assert.NoError(t, err)
		defer os.RemoveAll(topDir)

		// A unlikely to exist file name
		nonExistingFileName := strconv.FormatInt(time.Now().Unix(), 10)
		wantErrMsg := fmt.Sprintf(
			"unable to locate file specified (%s): reached root of file system",
			nonExistingFileName)

		_, err = findInParentTree(nonExistingFileName, topDir)
		assert.EqualError(t, err, wantErrMsg)
	})

	t.Run("returns a friendly error if file is an absolute path", func(t *testing.T) {
		topDir, err := ioutil.TempDir("", "summon")
		assert.NoError(t, err)
		defer os.RemoveAll(topDir)

		absFileName := "/foo/bar/baz"
		wantErrMsg := "file specified (/foo/bar/baz) is an absolute path: will not recurse up"

		_, err = findInParentTree(absFileName, topDir)
		assert.EqualError(t, err, wantErrMsg)
	})

	t.Run("returns a friendly error in unexpected circumstances (100% coverage)", func(t *testing.T) {
		topDir, err := ioutil.TempDir("", "summon")
		assert.NoError(t, err)
		defer os.RemoveAll(topDir)

		fileNameWithNulByte := "pizza\x00margherita"
		wantErrMsg := "unable to locate file specified (pizza\x00margherita): stat"

		_, err = findInParentTree(fileNameWithNulByte, topDir)
		assert.Contains(t, err.Error(), wantErrMsg)
	})
}

func TestReturnStatusOfError(t *testing.T) {
	t.Run("returns no error as 0", func(t *testing.T) {
		res, err := returnStatusOfError(nil)
		assert.NoError(t, err)
		assert.Equal(t, 0, res)
	})

	t.Run("returns ExitError as the wrapped exit status", func(t *testing.T) {
		exit := exec.Command("false").Run()
		res, err := returnStatusOfError(exit)
		assert.NoError(t, err)
		assert.Equal(t, 1, res)
	})

	t.Run("returns other errors unchanged", func(t *testing.T) {
		expected := errors.New("test")
		_, err := returnStatusOfError(expected)
		assert.Equal(t, expected, err)
	})
}
