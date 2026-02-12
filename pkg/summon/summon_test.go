package summon

import (
	"errors"
	"fmt"
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
	t.Run("Variable resolution", func(t *testing.T) {
		tests := []struct {
			name       string
			yamlInline string
			expected   string
		}{
			{"plain value", "FOO: valueOfVariable", "valueOfVariable"},
			{"default when provider returns empty", "FOO: !str:default='defaultValueOfVariable'", "defaultValueOfVariable"},
			{"provider value overrides default", "FOO: !str:default='something' valueOfVariable", "valueOfVariable"},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				dir := t.TempDir()
				tempFile := filepath.Join(dir, "outputFile.txt")

				code, err := RunSubprocess(&SubprocessConfig{
					Args:       []string{"bash", "-c", "echo -n \"$FOO\" > " + tempFile},
					YamlInline: tc.yamlInline,
				})

				assert.NoError(t, err)
				assert.Equal(t, 0, code)

				content, err := os.ReadFile(tempFile)
				assert.NoError(t, err)
				assert.Equal(t, tc.expected, string(content))
			})
		}
	})

	t.Run("Finds and uses secrets file in a directory above the working directory", func(t *testing.T) {
		topDir := t.TempDir()

		// Write a secrets.yml with a real secret that the subprocess will echo
		secretsPath := filepath.Join(topDir, "secrets.yml")
		err := os.WriteFile(secretsPath, []byte("BAR: barvalue\n"), 0o644)
		assert.NoError(t, err)

		// Create a deep subdirectory and chdir into it
		downDir := filepath.Join(topDir, "dir1", "dir2", "dir3")
		err = os.MkdirAll(downDir, 0o700)
		assert.NoError(t, err)
		t.Chdir(downDir)

		outFile := filepath.Join(topDir, "output.txt")
		code, err := RunSubprocess(&SubprocessConfig{
			Args:      []string{"bash", "-c", "echo -n \"$BAR\" > " + outFile},
			RecurseUp: true,
			Filepath:  "secrets.yml",
		})

		assert.NoError(t, err)
		assert.Equal(t, 0, code)

		content, err := os.ReadFile(outFile)
		assert.NoError(t, err)
		assert.Equal(t, "barvalue", string(content))
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
			resultsCh <- prov.Result{Key: expectedKey, Value: expectedValue, Error: nil}
			close(resultsCh)
		}()

		results, err := handleResultsFromProvider(resultsCh, errorsCh, filteredSecrets, &tempFactory)

		assert.NoError(t, err)
		assert.Equal(t, 1, len(results))
		assert.Equal(t, expectedKey, results[0].Key)
		assert.Equal(t, expectedValue, results[0].Value)
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
			resultsCh <- prov.Result{Key: expectedKey, Value: "", Error: nil}
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

	t.Run("Returns error result when formatForEnv fails", func(t *testing.T) {
		resultsCh := make(chan prov.Result, 1)
		errorsCh := make(chan error, 1)

		// Use an invalid path so tempFactory.Push() fails
		tempFactory := NewTempFactory("/nonexistent/dir")
		defer tempFactory.Cleanup()

		filteredSecrets := secretsyml.SecretsMap{
			"FILE_KEY": secretsyml.SecretSpec{
				Path: "some-value",
				Tags: []secretsyml.YamlTag{secretsyml.Var, secretsyml.File},
			},
		}

		go func() {
			resultsCh <- prov.Result{Key: "FILE_KEY", Value: "content", Error: nil}
			close(resultsCh)
		}()

		results, err := handleResultsFromProvider(resultsCh, errorsCh, filteredSecrets, &tempFactory)

		assert.NoError(t, err)
		assert.Len(t, results, 1)
		assert.Equal(t, "FILE_KEY", results[0].Key)
		assert.Empty(t, results[0].Value)
		assert.ErrorContains(t, results[0].Error, "/nonexistent/dir")
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
			{Key: nonVarKey, Value: nonVarPath, Error: nil},
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

	t.Run("Returns error result when formatForEnv fails for non-variable", func(t *testing.T) {
		tempFactory := NewTempFactory("/nonexistent/dir")
		defer tempFactory.Cleanup()

		secrets := secretsyml.SecretsMap{
			"FILE_KEY": secretsyml.SecretSpec{
				Path: "file-content",
				Tags: []secretsyml.YamlTag{secretsyml.File},
			},
		}

		results, filteredSecrets := filterNonVariables(secrets, &tempFactory)

		assert.Empty(t, filteredSecrets, "file specs should not be in filtered (var-only) secrets")
		assert.Len(t, results, 1)
		assert.Equal(t, "FILE_KEY", results[0].Key)
		assert.Empty(t, results[0].Value)
		assert.ErrorContains(t, results[0].Error, "/nonexistent/dir")
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

		output, err := convertSubsToMap(input)

		assert.NoError(t, err)
		assert.EqualValues(t, expected, output)
	})

	t.Run("Invalid substitutions produce error", func(t *testing.T) {
		input := []string{
			"invalidsubstitution",
		}

		output, err := convertSubsToMap(input)
		assert.ErrorContains(t, err, "invalid substitution format")
		assert.Nil(t, output)
	})
}

func TestFormatForEnvString(t *testing.T) {
	t.Run("formatForEnv should return a KEY=VALUE string that can be appended to an environment", func(t *testing.T) {
		t.Run("For variables, VALUE should be the value of the secret", func(t *testing.T) {
			spec := secretsyml.SecretSpec{
				Path: "mysql1/password",
				Tags: []secretsyml.YamlTag{secretsyml.Var},
			}
			k, v, err := formatForEnv(
				"dbpass",
				"mysecretvalue",
				spec,
				nil,
			)

			assert.NoError(t, err)
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
			key, path, err := formatForEnv(
				"SSL_CERT",
				"mysecretvalue",
				spec,
				&tempFactory,
			)

			assert.NoError(t, err)
			assert.Equal(t, "SSL_CERT", key)

			// Temp path should exist
			_, err = os.Stat(path)
			assert.NoError(t, err)

			contents, _ := os.ReadFile(path)

			assert.Equal(t, "mysecretvalue", string(contents))
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
	tests := []struct {
		name        string
		secrets     secretsyml.SecretsMap
		fetchSecret func(string) ([]byte, error)
		tempPath    string
		assertFunc  func(t *testing.T, results []prov.Result)
	}{
		{
			name: "returns results for all secrets",
			secrets: secretsyml.SecretsMap{
				"key1": secretsyml.SecretSpec{Path: "path1"},
				"key2": secretsyml.SecretSpec{Path: "path2"},
			},
			fetchSecret: func(path string) ([]byte, error) { return []byte(path), nil },
			assertFunc: func(t *testing.T, results []prov.Result) {
				assert.Len(t, results, 2)
				for _, r := range results {
					assert.NoError(t, r.Error)
					// The stub returns the path as the value, so value should equal the spec's path
					assert.NotEmpty(t, r.Value)
				}
			},
		},
		{
			name: "returns error when fetch fails",
			secrets: secretsyml.SecretsMap{
				"FAILING_KEY": {Path: "path/to/secret", Tags: []secretsyml.YamlTag{secretsyml.Var}},
			},
			fetchSecret: func(path string) ([]byte, error) {
				return nil, fmt.Errorf("provider error for %s", path)
			},
			assertFunc: func(t *testing.T, results []prov.Result) {
				assert.Len(t, results, 1)
				assert.Equal(t, "FAILING_KEY", results[0].Key)
				assert.Empty(t, results[0].Value)
				assert.ErrorContains(t, results[0].Error, "provider error for path/to/secret")
			},
		},
		{
			name: "returns error when formatForEnv fails",
			secrets: secretsyml.SecretsMap{
				"FILE_KEY": {Path: "path/to/secret", Tags: []secretsyml.YamlTag{secretsyml.Var, secretsyml.File}},
			},
			fetchSecret: func(path string) ([]byte, error) { return []byte("secret-content"), nil },
			tempPath:    "/nonexistent/dir",
			assertFunc: func(t *testing.T, results []prov.Result) {
				assert.Len(t, results, 1)
				assert.Equal(t, "FILE_KEY", results[0].Key)
				assert.Empty(t, results[0].Value)
				assert.ErrorContains(t, results[0].Error, "/nonexistent/dir")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sc := &SubprocessConfig{FetchSecret: tc.fetchSecret}
			tempFactory := NewTempFactory(tc.tempPath)
			defer tempFactory.Cleanup()

			results := nonInteractiveProviderFallback(tc.secrets, sc, &tempFactory)
			tc.assertFunc(t, results)
		})
	}
}
