package pushtofile

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	filetemplates "github.com/cyberark/summon/pkg/file_templates"
	"github.com/cyberark/summon/pkg/provider"
	"github.com/cyberark/summon/pkg/secretsyml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type pushToFileWithDepsTestCase struct {
	description            string
	file                   SecretFile
	overrideResults        []provider.Result // Overrides secret results generated from file secret specs
	overridePushToWriter   func(writer io.Writer, filePath string, fileTemplate string, fileSecrets []*filetemplates.Secret) error
	toWriterPusherErr      error
	toWriteCloserOpenerErr error
	assert                 func(t *testing.T,
		spyOpenWriteCloser openWriteCloserSpy,
		closableBuf *ClosableBuffer,
		spyPushToWriter pushToWriterSpy,
		err error)
}

func (tc *pushToFileWithDepsTestCase) Run(t *testing.T) {
	t.Run(tc.description, func(t *testing.T) {
		// Input
		file := tc.file

		// Setup mocks
		closableBuf := new(ClosableBuffer)
		spyPushToWriter := pushToWriterSpy{
			err: tc.toWriterPusherErr,
		}
		spyOpenWriteCloser := openWriteCloserSpy{
			writeCloser: closableBuf,
			err:         tc.toWriteCloserOpenerErr,
		}

		// Use secrets from file or override
		var results []provider.Result
		if tc.overrideResults != nil {
			results = tc.overrideResults
		} else {
			for alias := range file.SecretSpecs() {
				results = append(results, provider.Result{
					Key:   alias,
					Value: fmt.Sprintf("value-%s", file.SecretSpecs()[alias].Path),
				})
			}
		}

		pushToWriterFunc := spyPushToWriter.Call
		if tc.overridePushToWriter != nil {
			pushToWriterFunc = tc.overridePushToWriter
		}

		// Exercise
		_, err := file.writeWithDeps(
			spyOpenWriteCloser.Call,
			pushToWriterFunc,
			results)

		tc.assert(t, spyOpenWriteCloser, closableBuf, spyPushToWriter, err)
	})
}

func modifyGoodFile(modifiers ...func(SecretFile) SecretFile) SecretFile {
	file := SecretFile{
		FileConfig: secretsyml.FileConfig{
			Path:        "/absolute/path/to/file",
			Template:    "filetemplate",
			Format:      "template",
			Permissions: 0o123,
			Secrets:     goodSecretSpecs(),
		},
	}

	for _, modifier := range modifiers {
		file = modifier(file)
	}

	return file
}

func goodSecretSpecs() secretsyml.SecretsMap {
	return secretsyml.SecretsMap{
		"alias1": secretsyml.SecretSpec{Path: "path1"},
		"alias2": secretsyml.SecretSpec{Path: "path2"},
	}
}

// Helper to create provider results from a map
func createResults(keyValues map[string]string) []provider.Result {
	results := make([]provider.Result, 0, len(keyValues))
	for key, value := range keyValues {
		results = append(results, provider.Result{
			Key:   key,
			Value: value,
		})
	}
	return results
}

// Helper to create provider results with errors
func createResultsWithErrors(keyValues map[string]string, errorKeys map[string]error) []provider.Result {
	results := make([]provider.Result, 0, len(keyValues)+len(errorKeys))
	for key, value := range keyValues {
		results = append(results, provider.Result{
			Key:   key,
			Value: value,
		})
	}
	for key, err := range errorKeys {
		results = append(results, provider.Result{
			Key:   key,
			Value: "",
			Error: err,
		})
	}
	return results
}

// Helper to create expected secrets from a map
func expectedSecrets(keyValues map[string]string) []*filetemplates.Secret {
	secrets := make([]*filetemplates.Secret, 0, len(keyValues))
	for key, value := range keyValues {
		secrets = append(secrets, &filetemplates.Secret{
			Alias: key,
			Value: value,
		})
	}
	return secrets
}

// Helper to convert secret slice to map for comparison
func secretsToMap(secrets []*filetemplates.Secret) map[string]string {
	m := make(map[string]string, len(secrets))
	for _, secret := range secrets {
		m[secret.Alias] = secret.Value
	}
	return m
}

// Helper assertion for successful write operations
func assertSuccessfulWrite(expectedPath, expectedTemplate string, expectedSecrets []*filetemplates.Secret, expectedPerms os.FileMode) func(*testing.T, openWriteCloserSpy, *ClosableBuffer, pushToWriterSpy, error) {
	return func(t *testing.T, spyOpenWriteCloser openWriteCloserSpy, closableBuf *ClosableBuffer, spyPushToWriter pushToWriterSpy, err error) {
		assert.NoError(t, err)
		assert.Equal(t, closableBuf, spyPushToWriter.args.writer)
		assert.Equal(t, expectedPath, spyPushToWriter.args.filePath)
		assert.Equal(t, expectedTemplate, spyPushToWriter.args.fileTemplate)
		// Compare secrets in order-independent way due to map iteration
		assert.Len(t, spyPushToWriter.args.fileSecrets, len(expectedSecrets))
		assert.Equal(t, secretsToMap(expectedSecrets), secretsToMap(spyPushToWriter.args.fileSecrets))
		// openWriteCloser receives the absolute path
		assert.Equal(t, expectedPerms, spyOpenWriteCloser.args.permissions)
		assert.True(t, filepath.IsAbs(spyOpenWriteCloser.args.path), "openWriteCloser should receive absolute path")
	}
}

// Helper assertion for error cases
func assertWriteErrorContains(expectedMsg string) func(*testing.T, openWriteCloserSpy, *ClosableBuffer, pushToWriterSpy, error) {
	return func(t *testing.T, _ openWriteCloserSpy, _ *ClosableBuffer, _ pushToWriterSpy, err error) {
		if assert.Error(t, err) {
			assert.Contains(t, err.Error(), expectedMsg)
		}
	}
}

var pushToFileWithDepsTestCases = []pushToFileWithDepsTestCase{
	{
		description:          "happy path",
		file:                 modifyGoodFile(),
		overrideResults:      nil,
		overridePushToWriter: nil,
		assert: assertSuccessfulWrite(
			"/absolute/path/to/file",
			"filetemplate",
			expectedSecrets(map[string]string{
				"alias1": "value-path1",
				"alias2": "value-path2",
			}),
			0o123,
		),
	},
	{
		description: "happy path with relative file path",
		file: modifyGoodFile(func(file SecretFile) SecretFile {
			file.FileConfig.Path = "path/to/file"
			return file
		}),
		overrideResults:      nil,
		overridePushToWriter: nil,
		assert: func(t *testing.T, spyOpenWriteCloser openWriteCloserSpy, closableBuf *ClosableBuffer, spyPushToWriter pushToWriterSpy, err error) {
			assert.NoError(t, err)
			assert.Equal(t, "path/to/file", spyPushToWriter.args.filePath)
			assert.Equal(t, "filetemplate", spyPushToWriter.args.fileTemplate)
			// Check secrets are present (order may vary due to map iteration)
			assert.Len(t, spyPushToWriter.args.fileSecrets, 2)
			secretMap := secretsToMap(spyPushToWriter.args.fileSecrets)
			assert.Equal(t, "value-path1", secretMap["alias1"])
			assert.Equal(t, "value-path2", secretMap["alias2"])
			pwd, err := os.Getwd()
			require.NoError(t, err)
			assert.Equal(t,
				openWriteCloserArgs{
					path:        pwd + "/path/to/file",
					permissions: 0o123,
					overwrite:   false,
				},
				spyOpenWriteCloser.args,
			)
		},
	},
	{
		description: "missing file format or template",
		file: modifyGoodFile(func(file SecretFile) SecretFile {
			file.FileConfig.Template = ""
			file.FileConfig.Format = ""

			return file
		}),
		overrideResults: nil,
		assert: func(t *testing.T, spyOpenWriteCloser openWriteCloserSpy, closableBuf *ClosableBuffer, spyPushToWriter pushToWriterSpy, err error) {
			if assert.NoError(t, err) {
				// Defaults to yaml
				assert.Equal(t, yamlTemplate, spyPushToWriter.args.fileTemplate)
			}
		},
	},
	{
		description:     "secrets list is empty",
		file:            modifyGoodFile(),
		overrideResults: []provider.Result{},
		assert:          assertWriteErrorContains(`some secret specs are not present in secrets`),
	},
	{
		description: "file template precedence",
		file: modifyGoodFile(func(file SecretFile) SecretFile {
			file.FileConfig.Template = "setfiletemplate"
			file.FileConfig.Format = "json"

			return file
		}),
		overrideResults: nil,
		assert: func(t *testing.T, spyOpenWriteCloser openWriteCloserSpy, closableBuf *ClosableBuffer, spyPushToWriter pushToWriterSpy, err error) {
			if assert.NoError(t, err) {
				assert.Equal(t, `setfiletemplate`, spyPushToWriter.args.fileTemplate)
			}
		},
	},
	{
		description:     "template execution error",
		file:            modifyGoodFile(),
		overrideResults: nil,
		overridePushToWriter: func(writer io.Writer, filePath string, fileTemplate string, fileSecrets []*filetemplates.Secret) error {
			return errors.New("underlying error message")
		},
		assert: func(t *testing.T, spyOpenWriteCloser openWriteCloserSpy, closableBuf *ClosableBuffer, spyPushToWriter pushToWriterSpy, err error) {
			if assert.Error(t, err) {
				assert.Contains(t, err.Error(), `failed to execute template, with secret values, on push to file`)
				assert.NotContains(t, err.Error(), "underlying error message")
			}
		},
	},
	{
		description:     "template execution panic",
		file:            modifyGoodFile(),
		overrideResults: nil,
		overridePushToWriter: func(writer io.Writer, filePath string, fileTemplate string, fileSecrets []*filetemplates.Secret) error {
			panic("canned panic response - maybe containing secrets")
		},
		assert: func(t *testing.T, spyOpenWriteCloser openWriteCloserSpy, closableBuf *ClosableBuffer, spyPushToWriter pushToWriterSpy, err error) {
			if assert.Error(t, err) {
				assert.Contains(t, err.Error(), `failed to execute template, with secret values, on push to file`)
				assert.NotContains(t, err.Error(), "canned panic response - maybe containing secrets")
			}
		},
	},
	{
		description: "provider error without ignore",
		file:        modifyGoodFile(),
		overrideResults: createResultsWithErrors(
			map[string]string{"alias1": "value1"},
			map[string]error{"alias2": errors.New("secret fetch failed")},
		),
		assert: assertWriteErrorContains("Error fetching secret: secret fetch failed"),
	},
	{
		description: "provider error with IgnoreAll",
		file: modifyGoodFile(func(file SecretFile) SecretFile {
			file.IgnoreAll = true
			return file
		}),
		overrideResults: createResultsWithErrors(
			map[string]string{"alias1": "value1", "alias2": "value2"},
			map[string]error{"alias3": errors.New("ignored error")},
		),
		assert: assertSuccessfulWrite(
			"/absolute/path/to/file",
			"filetemplate",
			expectedSecrets(map[string]string{
				"alias1": "value1",
				"alias2": "value2",
			}),
			0o123,
		),
	},
	{
		description: "provider error with specific ignore",
		file: modifyGoodFile(func(file SecretFile) SecretFile {
			file.FileConfig.Secrets = secretsyml.SecretsMap{
				"alias1": secretsyml.SecretSpec{Path: "path1"},
				"alias2": secretsyml.SecretSpec{Path: "path2"},
				"alias3": secretsyml.SecretSpec{Path: "path3"},
			}
			file.Ignores = []string{"alias3"}
			file.FileConfig.Permissions = 0
			return file
		}),
		overrideResults: createResultsWithErrors(
			map[string]string{
				"alias1": "value1",
				"alias2": "value2",
			},
			map[string]error{"alias3": errors.New("ignored specific error")},
		),
		assert: assertSuccessfulWrite(
			"/absolute/path/to/file",
			"filetemplate",
			expectedSecrets(map[string]string{
				"alias1": "value1",
				"alias2": "value2",
			}),
			defaultFilePermissions,
		),
	},
	{
		description: "multiple ignores",
		file: modifyGoodFile(func(file SecretFile) SecretFile {
			file.FileConfig.Secrets = secretsyml.SecretsMap{
				"alias1": secretsyml.SecretSpec{Path: "path1"},
				"alias2": secretsyml.SecretSpec{Path: "path2"},
				"alias3": secretsyml.SecretSpec{Path: "path3"},
			}
			file.Ignores = []string{"alias2", "alias3"}
			return file
		}),
		overrideResults: createResultsWithErrors(
			map[string]string{
				"alias1": "value1",
			},
			map[string]error{
				"alias2": errors.New("ignored error 1"),
				"alias3": errors.New("ignored error 2"),
			},
		),
		assert: assertSuccessfulWrite(
			"/absolute/path/to/file",
			"filetemplate",
			expectedSecrets(map[string]string{
				"alias1": "value1",
			}),
			0o123,
		),
	},
	{
		description: "multiple provider errors returns first",
		file:        modifyGoodFile(),
		overrideResults: createResultsWithErrors(
			map[string]string{},
			map[string]error{
				"alias1": errors.New("first error"),
				"alias2": errors.New("second error"),
			},
		),
		assert: func(t *testing.T, _ openWriteCloserSpy, _ *ClosableBuffer, _ pushToWriterSpy, err error) {
			if assert.Error(t, err) {
				assert.Contains(t, err.Error(), "Error fetching secret:")
				// Should contain one of the errors (order not guaranteed due to map iteration)
				assert.True(t, strings.Contains(err.Error(), "first error") || strings.Contains(err.Error(), "second error"))
			}
		},
	},
	{
		description: "result path not in specs prints warning",
		file:        modifyGoodFile(),
		overrideResults: []provider.Result{
			{Key: "alias1", Value: "value1"},
			{Key: "alias2", Value: "value2"},
			{Key: "unknown-alias", Value: "unknown-value"},
		},
		assert: assertSuccessfulWrite(
			"/absolute/path/to/file",
			"filetemplate",
			expectedSecrets(map[string]string{
				"alias1": "value1",
				"alias2": "value2",
			}),
			0o123,
		),
	},
	{
		description: "overwrite true is passed to openWriteCloser",
		file: modifyGoodFile(func(file SecretFile) SecretFile {
			file.FileConfig.Overwrite = true
			return file
		}),
		assert: func(t *testing.T, spyOpenWriteCloser openWriteCloserSpy, _ *ClosableBuffer, _ pushToWriterSpy, err error) {
			assert.NoError(t, err)
			assert.True(t, spyOpenWriteCloser.args.overwrite, "overwrite should be true")
		},
	},
	{
		description: "overwrite false (default) is passed to openWriteCloser",
		file:        modifyGoodFile(),
		assert: func(t *testing.T, spyOpenWriteCloser openWriteCloserSpy, _ *ClosableBuffer, _ pushToWriterSpy, err error) {
			assert.NoError(t, err)
			assert.False(t, spyOpenWriteCloser.args.overwrite, "overwrite should be false by default")
		},
	},
}

func TestSecretFile_pushToFileWithDeps(t *testing.T) {
	for _, tc := range pushToFileWithDepsTestCases {
		tc.Run(t)
	}

	for _, format := range []string{"json", "yaml", "bash", "dotenv"} {
		tc := pushToFileWithDepsTestCase{
			description: fmt.Sprintf("%s format", format),
			file: modifyGoodFile(func(file SecretFile) SecretFile {
				file.FileConfig.Template = ""
				file.FileConfig.Format = format

				return file
			}),
			assert: func(t *testing.T, _ openWriteCloserSpy, _ *ClosableBuffer, spyPushToWriter pushToWriterSpy, err error) {
				if assert.NoError(t, err) {
					assert.Equal(t, standardTemplates[format].template, spyPushToWriter.args.fileTemplate)
				}
			},
		}

		tc.Run(t)
	}
}

func TestSecretFile_Write(t *testing.T) {
	// Create temp directory
	dir, err := os.MkdirTemp("", "")
	assert.NoError(t, err)
	defer os.Remove(dir)

	// Common test data
	commonResults := createResults(map[string]string{
		"alias1": "value1",
		"alias2": "value2",
	})

	for _, tc := range []struct {
		description     string
		path            string
		filePermissions os.FileMode
	}{
		{"file with existing parent folder, perms 0660", "./file", 0660},
		{"file with existing parent folder, perms 0744", "./file", 0744},
		{"file with non-existent parent folder", "./path/to/file", 0640},
	} {
		t.Run(tc.description, func(t *testing.T) {
			absoluteFilePath := filepath.Join(dir, tc.path)

			// Create a file, and push to file
			file := SecretFile{
				FileConfig: secretsyml.FileConfig{
					Path:        absoluteFilePath,
					Template:    "",
					Format:      "yaml",
					Permissions: tc.filePermissions,
					Secrets:     goodSecretSpecs(),
					Overwrite:   true, // Allow overwrite since multiple tests write to same path
				},
			}
			_, err = file.Write(commonResults)
			assert.NoError(t, err)

			// Read file contents and metadata
			contentBytes, err := os.ReadFile(absoluteFilePath)
			assert.NoError(t, err)
			f, err := os.Stat(absoluteFilePath)
			assert.NoError(t, err)

			// Assert on file contents and metadata
			assert.EqualValues(t, f.Mode(), tc.filePermissions)
			content := string(contentBytes)
			assert.Contains(t, content, `"alias1": "value1"`)
			assert.Contains(t, content, `"alias2": "value2"`)
		})
	}

	t.Run("failure to mkdir", func(t *testing.T) {
		file := SecretFile{
			FileConfig: secretsyml.FileConfig{
				Path:        "/dev/stdout/x",
				Template:    "",
				Format:      "yaml",
				Permissions: 0x744,
				Secrets:     goodSecretSpecs(),
			},
		}
		_, err = file.Write(commonResults)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unable to mkdir")
	})

	t.Run("failure to open file", func(t *testing.T) {
		file := SecretFile{
			FileConfig: secretsyml.FileConfig{
				Path:        "/",
				Template:    "",
				Format:      "yaml",
				Permissions: 0x744,
				Secrets:     goodSecretSpecs(),
			},
		}
		_, err = file.Write(commonResults)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "must contain a filename")
	})

	t.Run("overwrite false errors when file exists", func(t *testing.T) {
		filePath := filepath.Join(dir, "overwrite-test-no")

		// Create the file first
		err := os.WriteFile(filePath, []byte("existing content"), 0o644)
		require.NoError(t, err)
		defer os.Remove(filePath)

		file := SecretFile{
			FileConfig: secretsyml.FileConfig{
				Path:      filePath,
				Format:    "yaml",
				Secrets:   goodSecretSpecs(),
				Overwrite: false,
			},
		}
		_, err = file.Write(commonResults)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already exists")
		assert.Contains(t, err.Error(), "overwrite is not enabled")

		// Original content should be unchanged
		content, _ := os.ReadFile(filePath)
		assert.Equal(t, "existing content", string(content))
	})

	t.Run("overwrite true succeeds when file exists", func(t *testing.T) {
		filePath := filepath.Join(dir, "overwrite-test-yes")

		// Create the file first
		err := os.WriteFile(filePath, []byte("old content"), 0o644)
		require.NoError(t, err)
		defer os.Remove(filePath)

		file := SecretFile{
			FileConfig: secretsyml.FileConfig{
				Path:        filePath,
				Format:      "yaml",
				Permissions: 0o644,
				Secrets:     goodSecretSpecs(),
				Overwrite:   true,
			},
		}
		_, err = file.Write(commonResults)
		assert.NoError(t, err)

		// Content should be updated
		content, _ := os.ReadFile(filePath)
		assert.Contains(t, string(content), `"alias1": "value1"`)
		assert.Contains(t, string(content), `"alias2": "value2"`)
	})

	t.Run("overwrite false succeeds when file does not exist", func(t *testing.T) {
		filePath := filepath.Join(dir, "overwrite-test-new")
		defer os.Remove(filePath)

		file := SecretFile{
			FileConfig: secretsyml.FileConfig{
				Path:        filePath,
				Format:      "yaml",
				Permissions: 0o644,
				Secrets:     goodSecretSpecs(),
				Overwrite:   false,
			},
		}
		_, err := file.Write(commonResults)
		assert.NoError(t, err)

		// File should be created with correct content
		content, _ := os.ReadFile(filePath)
		assert.Contains(t, string(content), `"alias1": "value1"`)
		assert.Contains(t, string(content), `"alias2": "value2"`)
	})
}

func TestSecretFile_absoluteFilePath(t *testing.T) {
	pwd, _ := os.Getwd()

	testCases := []struct {
		name     string
		path     string
		errorMsg string
		assertFn func(*testing.T, string)
	}{
		{
			name: "absolute path with valid filename",
			path: "/tmp/file",
			assertFn: func(t *testing.T, result string) {
				assert.Equal(t, "/tmp/file", result)
			},
		},
		{
			name: "relative path",
			path: "relative/file",
			assertFn: func(t *testing.T, result string) {
				assert.Equal(t, filepath.Join(pwd, "relative/file"), result)
				assert.True(t, filepath.IsAbs(result))
			},
		},
		{
			name:     "path ends with slash",
			path:     "/tmp/",
			errorMsg: "must contain a filename",
		},
		{
			name:     "empty path",
			path:     "",
			errorMsg: "must contain a filename",
		},
		{
			name:     "filename exceeds max length",
			path:     "/tmp/" + strings.Repeat("a", maxFilenameLen+1),
			errorMsg: "must not be longer than",
		},
		{
			name: "filename at max length",
			path: "/tmp/" + strings.Repeat("a", maxFilenameLen),
			assertFn: func(t *testing.T, result string) {
				assert.Equal(t, "/tmp/"+strings.Repeat("a", maxFilenameLen), result)
			},
		},
		{
			name: "nested relative path",
			path: "a/b/c/file",
			assertFn: func(t *testing.T, result string) {
				assert.Equal(t, filepath.Join(pwd, "a/b/c/file"), result)
			},
		},
		{
			name: "path traversal is cleaned",
			path: "a/../b/file",
			assertFn: func(t *testing.T, result string) {
				assert.Equal(t, filepath.Join(pwd, "b/file"), result)
				assert.NotContains(t, result, "..")
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			file := SecretFile{
				FileConfig: secretsyml.FileConfig{
					Path: tc.path,
				},
			}
			result, err := file.absoluteFilePath()

			if tc.errorMsg != "" {
				assert.ErrorContains(t, err, tc.errorMsg)
			} else {
				assert.NoError(t, err)
				if tc.assertFn != nil {
					tc.assertFn(t, result)
				}
			}
		})
	}
}
