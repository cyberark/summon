package summon

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	prov "github.com/cyberark/summon/pkg/provider"
	"github.com/cyberark/summon/pkg/secretsyml"
	"github.com/stretchr/testify/assert"
)

func TestRunSubprocess(t *testing.T) {
	t.Run("Variable resolution correctly resolves variables", func(t *testing.T) {
		expectedValue := "valueOfVariable"

		dir, err := os.MkdirTemp("", "summon")
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

		content, err := os.ReadFile(tempFile)
		assert.NoError(t, err)
		if err != nil {
			return
		}

		assert.Equal(t, expectedValue, string(content))
	})

	t.Run("Finds secrets file in a directory above the working directory", func(t *testing.T) {
		var err error
		topDir := t.TempDir()

		fileAbovePath := filepath.Join(topDir, "secrets.yml")
		_, err = os.Create(fileAbovePath)
		assert.NoError(t, err)

		// Create a downwards directory hierarchy, starting from topDir, and
		// chdir to it.
		downDir := filepath.Join(topDir, "dir1", "dir2", "dir3")
		err = os.MkdirAll(downDir, 0o700)
		assert.NoError(t, err)
		t.Chdir(downDir)

		code, err := RunSubprocess(&SubprocessConfig{
			Args:      []string{"true"},
			RecurseUp: true,
			Filepath:  "secrets.yml",
		})

		assert.NoError(t, err)
		assert.Equal(t, 0, code)
	})
}

func TestHandleResultsFromProvider(t *testing.T) {
	t.Run("Returns results when provider returns results", func(t *testing.T) {
		secretPath := "path/to/secret"
		expectedValue := "secretvalue"
		expectedKey := "SERVICE_KEY"
		resultsCh := make(chan prov.Result)
		errorsCh := make(chan error, 1)

		tempFactory := NewTempFactory("")
		defer tempFactory.Cleanup()

		filteredSecrets := secretsyml.SecretsMap{
			expectedKey: secretsyml.SecretSpec{
				Path:         secretPath,
				DefaultValue: "",
				Tags:         []secretsyml.YamlTag{secretsyml.Var},
			},
		}

		go func() {
			resultsCh <- prov.Result{expectedKey, expectedValue, nil}
			close(resultsCh)
		}()

		results, err := handleResultsFromProvider(resultsCh, errorsCh, filteredSecrets, &tempFactory)

		assert.NoError(t, err)
		assert.Equal(t, 1, len(results))
		assert.Equal(t, expectedKey, results[0].Key)
		assert.Equal(t, expectedValue, results[0].Value)
	})
	t.Run("Handles large number of results", func(t *testing.T) {
		numResults := 1000
		resultsCh := make(chan prov.Result, numResults)
		errorsCh := make(chan error, 1)

		tempFactory := NewTempFactory("")
		defer tempFactory.Cleanup()

		filteredSecrets := make(secretsyml.SecretsMap, numResults)
		for i := range numResults {
			key := fmt.Sprintf("SECRET_KEY_%d", i)
			filteredSecrets[key] = secretsyml.SecretSpec{
				Path:         key,
				DefaultValue: "",
				Tags:         []secretsyml.YamlTag{secretsyml.Var},
			}
		}

		go func() {
			for i := range numResults {
				key := fmt.Sprintf("SECRET_KEY_%d", i)
				// Generate a 1024 character random string
				val := generateRandomString(1024)
				val = fmt.Sprintf("%s_____%d", val, i) // Append index
				resultsCh <- prov.Result{Key: key, Value: val, Error: nil}
			}
			close(resultsCh)
		}()

		results, err := handleResultsFromProvider(resultsCh, errorsCh, filteredSecrets, &tempFactory)

		assert.NoError(t, err)
		assert.Equal(t, numResults, len(results))
		for i := range numResults {
			expectedKey := fmt.Sprintf("SECRET_KEY_%d", i)
			expectedValue := fmt.Sprintf("_____%d", i)
			assert.Equal(t, expectedKey, results[i].Key)
			assert.Contains(t, results[i].Value, expectedValue)
		}
	})

	t.Run("Returns default value when provider returns empty value", func(t *testing.T) {
		secretPath := "path/to/secret"
		expectedValue := "defaultVal"
		expectedKey := "SERVICE_KEY"
		resultsCh := make(chan prov.Result)
		errorsCh := make(chan error, 1)

		tempFactory := NewTempFactory("")
		defer tempFactory.Cleanup()

		filteredSecrets := secretsyml.SecretsMap{
			expectedKey: secretsyml.SecretSpec{
				Path:         secretPath,
				DefaultValue: expectedValue,
				Tags:         []secretsyml.YamlTag{secretsyml.Var},
			},
		}

		go func() {
			resultsCh <- prov.Result{expectedKey, "", nil}
			close(resultsCh)
		}()

		results, err := handleResultsFromProvider(resultsCh, errorsCh, filteredSecrets, &tempFactory)

		assert.NoError(t, err)
		assert.Equal(t, 1, len(results))
		assert.Equal(t, expectedKey, results[0].Key)
		assert.Equal(t, expectedValue, results[0].Value)
	})

	t.Run("Returns error when provider cannot handle interactive mode", func(t *testing.T) {
		resultsCh := make(chan prov.Result, 1)
		errorsCh := make(chan error, 1)

		tempFactory := NewTempFactory("")
		defer tempFactory.Cleanup()

		filteredSecrets := secretsyml.SecretsMap{
			"SERVICE_KEY": secretsyml.SecretSpec{
				Path:         "path/to/secret",
				DefaultValue: "",
				Tags:         []secretsyml.YamlTag{secretsyml.Var}},
		}

		errorsCh <- prov.ErrInteractiveModeNotSupported

		results, err := handleResultsFromProvider(resultsCh, errorsCh, filteredSecrets, &tempFactory)

		assert.Error(t, err)
		assert.Equal(t, prov.ErrInteractiveModeNotSupported, err)
		assert.Nil(t, results)
	})
}

func TestFilterNonVariables(t *testing.T) {
	t.Run("Returns expected results and filtered secrets", func(t *testing.T) {
		varKey := "varKey"
		varPath := "varPath"
		nonVarKey := "nonVarKey"
		nonVarPath := "nonVarPath"

		tempFactory := NewTempFactory("")
		defer tempFactory.Cleanup()

		secrets := secretsyml.SecretsMap{
			varKey: secretsyml.SecretSpec{
				Path: varPath,
				Tags: []secretsyml.YamlTag{secretsyml.Var},
			},
			nonVarKey: secretsyml.SecretSpec{
				Path: nonVarPath,
				Tags: []secretsyml.YamlTag{secretsyml.Literal},
			},
		}

		expectedResults := []prov.Result{
			{nonVarKey, nonVarPath, nil},
		}

		expectedFilteredSecrets := secretsyml.SecretsMap{
			varKey: secretsyml.SecretSpec{
				Path: varPath,
				Tags: []secretsyml.YamlTag{secretsyml.Var},
			},
		}

		results, filteredSecrets := filterNonVariables(secrets, &tempFactory)

		assert.Equal(t, expectedResults, results)
		assert.Equal(t, expectedFilteredSecrets, filteredSecrets)
	})
}

func TestDefaultVariableResolution(t *testing.T) {
	t.Run("Variable resolution correctly resolves variables", func(t *testing.T) {
		expectedDefaultValue := "defaultValueOfVariable"

		dir, err := os.MkdirTemp("", "summon")
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

		content, err := os.ReadFile(tempFile)
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

		dir, err := os.MkdirTemp("", "summon")
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

		content, err := os.ReadFile(tempFile)
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

			contents, _ := os.ReadFile(path)

			assert.Contains(t, string(contents), "mysecretvalue")
		})
	})
}

func TestJoinEnv(t *testing.T) {
	t.Run("adds a trailing newline", func(t *testing.T) {
		result := joinEnv(map[string]string{"foo": "bar", "baz": "qux"})
		assert.Equal(t, "baz=qux\nfoo=bar\n", result)
	})

	t.Run("quotes values with spaces", func(t *testing.T) {
		result := joinEnv(map[string]string{"key": "value with spaces"})
		assert.Equal(t, "key=\"value with spaces\"\n", result)
	})

	t.Run("quotes and escapes multi-line values", func(t *testing.T) {
		multiLineValue := "-----BEGIN KEY-----\nCERT_DATA...\n-----END KEY-----"
		result := joinEnv(map[string]string{"CERT": multiLineValue})
		expected := "CERT=\"-----BEGIN KEY-----\\nCERT_DATA...\\n-----END KEY-----\"\n"
		assert.Equal(t, expected, result)
	})

	t.Run("escapes quotes in values", func(t *testing.T) {
		result := joinEnv(map[string]string{"key": "value with \"quotes\""})
		assert.Equal(t, "key=\"value with \\\"quotes\\\"\"\n", result)
	})

	t.Run("escapes backslashes in values", func(t *testing.T) {
		result := joinEnv(map[string]string{"key": "value\\with\\backslashes"})
		assert.Equal(t, "key=\"value\\\\with\\\\backslashes\"\n", result)
	})

	t.Run("handles mixed simple and complex values", func(t *testing.T) {
		result := joinEnv(map[string]string{
			"SIMPLE":  "value",
			"COMPLEX": "value with spaces",
		})
		assert.Contains(t, result, "SIMPLE=value\n")
		assert.Contains(t, result, "COMPLEX=\"value with spaces\"\n")
	})
}

func TestLocateFileRecurseUp(t *testing.T) {
	filename := "test.txt"

	t.Run("Finds file in current working directory", func(t *testing.T) {
		topDir := t.TempDir()

		localFilePath := filepath.Join(topDir, filename)
		_, err := os.Create(localFilePath)
		assert.NoError(t, err)

		gotPath, err := findInParentTree(filename, topDir)
		assert.NoError(t, err)

		assert.Equal(t, localFilePath, gotPath)
	})

	t.Run("Finds file in a directory above the working directory", func(t *testing.T) {
		topDir := t.TempDir()

		fileAbovePath := filepath.Join(topDir, filename)
		_, err := os.Create(fileAbovePath)
		assert.NoError(t, err)

		// Create a downwards directory hierarchy, starting from topDir
		downDir := filepath.Join(topDir, "dir1", "dir2", "dir3")
		err = os.MkdirAll(downDir, 0o700)
		assert.NoError(t, err)

		gotPath, err := findInParentTree(filename, downDir)
		assert.NoError(t, err)

		assert.Equal(t, fileAbovePath, gotPath)
	})

	t.Run("returns a friendly error if file not found", func(t *testing.T) {
		topDir := t.TempDir()

		// A unlikely to exist file name
		nonExistingFileName := strconv.FormatInt(time.Now().Unix(), 10)
		wantErrMsg := fmt.Sprintf(
			"unable to locate file specified (%s): reached root of file system",
			nonExistingFileName)

		_, err := findInParentTree(nonExistingFileName, topDir)
		assert.EqualError(t, err, wantErrMsg)
	})

	t.Run("returns a friendly error if file is an absolute path", func(t *testing.T) {
		topDir := t.TempDir()

		absFileName := "/foo/bar/baz"
		wantErrMsg := "file specified (/foo/bar/baz) is an absolute path: will not recurse up"

		_, err := findInParentTree(absFileName, topDir)
		assert.EqualError(t, err, wantErrMsg)
	})

	t.Run("returns a friendly error in unexpected circumstances (100% coverage)", func(t *testing.T) {
		topDir := t.TempDir()

		fileNameWithNulByte := "pizza\x00margherita"
		wantErrMsg := "unable to locate file specified (pizza\x00margherita): stat"

		_, err := findInParentTree(fileNameWithNulByte, topDir)
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

func TestNonInteractiveProviderFallback(t *testing.T) {
	secrets := secretsyml.SecretsMap{
		"key1": secretsyml.SecretSpec{Path: "path1"},
		"key2": secretsyml.SecretSpec{Path: "path2"},
	}
	sc := &SubprocessConfig{
		FetchSecret: func(path string) ([]byte, error) {
			return []byte(path), nil
		},
	}
	tempFactory := NewTempFactory("")
	defer tempFactory.Cleanup()

	results := nonInteractiveProviderFallback(secrets, sc, &tempFactory)

	assert.Equal(t, len(secrets), len(results))
	for _, result := range results {
		assert.Equal(t, secrets[result.Key].Path, result.Value)
		assert.Nil(t, result.Error)
	}
}

func generateRandomString(n int) string {
	const letters = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz-"
	ret := make([]byte, n)
	for i := range n {
		num, _ := rand.Int(rand.Reader, big.NewInt(int64(len(letters))))
		ret[i] = letters[num.Int64()]
	}

	return string(ret)
}
