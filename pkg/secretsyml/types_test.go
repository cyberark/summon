package secretsyml

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestYamlTag_String(t *testing.T) {
	tests := []struct {
		name     string
		tag      YamlTag
		expected string
	}{
		{"File tag", File, "File"},
		{"Var tag", Var, "Var"},
		{"Literal tag", Literal, "Literal"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.tag.String())
		})
	}
}

func TestYamlTag_String_Panic(t *testing.T) {
	assert.Panics(t, func() {
		_ = YamlTag(99).String()
	})
}

func TestFileConfig_Validate(t *testing.T) {
	tests := []struct {
		name   string
		config FileConfig
		errMsg string
	}{
		{
			name:   "Valid config",
			config: FileConfig{Path: "/path/to/file"},
		},
		{
			name:   "Missing path",
			config: FileConfig{},
			errMsg: "file config is missing required 'path' field",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.errMsg != "" {
				assert.EqualError(t, err, tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestParsedConfig_HasEnvSecrets(t *testing.T) {
	tests := []struct {
		name     string
		config   ParsedConfig
		expected bool
	}{
		{
			name:     "No env secrets",
			config:   ParsedConfig{EnvSecrets: SecretsMap{}},
			expected: false,
		},
		{
			name:     "Nil env secrets",
			config:   ParsedConfig{},
			expected: false,
		},
		{
			name: "Has env secrets",
			config: ParsedConfig{
				EnvSecrets: SecretsMap{
					"KEY": {Path: "path", Tags: []YamlTag{Var}},
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.config.HasEnvSecrets())
		})
	}
}

func TestParsedConfig_HasFileSecrets(t *testing.T) {
	tests := []struct {
		name     string
		config   ParsedConfig
		expected bool
	}{
		{
			name:     "No files",
			config:   ParsedConfig{},
			expected: false,
		},
		{
			name: "Has files",
			config: ParsedConfig{
				Files: []FileConfig{{Path: "/tmp/f"}},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.config.HasFileSecrets())
		})
	}
}

func TestParsedConfig_FileSecrets(t *testing.T) {
	tests := []struct {
		name         string
		config       ParsedConfig
		expectedKeys []string
	}{
		{
			name:         "No files",
			config:       ParsedConfig{},
			expectedKeys: nil,
		},
		{
			name: "Single file",
			config: ParsedConfig{
				Files: []FileConfig{
					{Secrets: SecretsMap{
						"A": {Path: "a/path", Tags: []YamlTag{Var}},
					}},
				},
			},
			expectedKeys: []string{"A"},
		},
		{
			name: "Multiple files merged",
			config: ParsedConfig{
				Files: []FileConfig{
					{Secrets: SecretsMap{
						"A": {Path: "a/path", Tags: []YamlTag{Var}},
					}},
					{Secrets: SecretsMap{
						"B": {Path: "b/path", Tags: []YamlTag{File}},
					}},
				},
			},
			expectedKeys: []string{"A", "B"},
		},
		{
			name: "Duplicate keys across files",
			config: ParsedConfig{
				Files: []FileConfig{
					{Secrets: SecretsMap{
						"X": {Path: "first", Tags: []YamlTag{Var}},
					}},
					{Secrets: SecretsMap{
						"X": {Path: "second", Tags: []YamlTag{Literal}},
					}},
				},
			},
			expectedKeys: []string{"X"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.FileSecrets()
			assert.Len(t, result, len(tt.expectedKeys))
			for _, key := range tt.expectedKeys {
				assert.Contains(t, result, key)
			}
		})
	}

	// Verify the duplicate-key case picks the last value
	t.Run("Duplicate key preserves last value", func(t *testing.T) {
		config := ParsedConfig{
			Files: []FileConfig{
				{Secrets: SecretsMap{"X": {Path: "first"}}},
				{Secrets: SecretsMap{"X": {Path: "second"}}},
			},
		}
		assert.Equal(t, "second", config.FileSecrets()["X"].Path)
	})
}
