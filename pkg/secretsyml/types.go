// Package secretsyml provides functions for parsing a string or file
// in secrets.yml format.
package secretsyml

import (
	"fmt"
	"maps"
	"os"
	"slices"

	"gopkg.in/yaml.v3"
)

// commonSections lists section names that are merged into every environment.
var commonSections = []string{"common", "default"}

// YamlTag represents the type annotation on a secrets.yml value.
type YamlTag uint8

const (
	File YamlTag = iota
	Var
	Literal
)

func (t YamlTag) String() string {
	switch t {
	case File:
		return "File"
	case Var:
		return "Var"
	case Literal:
		return "Literal"
	default:
		panic("unreachable!")
	}
}

// SecretSpec is a parsed secrets.yml entry describing *what* to fetch.
// It captures the YAML tag metadata (e.g. !var, !file) and the provider
// path but intentionally has no Value field — the actual secret content
// is only known after the provider is called (see provider.Result).
type SecretSpec struct {
	Tags         []YamlTag // How to treat the value: variable lookup, file, or literal.
	Path         string    // Provider path to fetch, or a literal value.
	DefaultValue string    // Fallback if the provider returns an empty string.
}

func (spec *SecretSpec) IsFile() bool {
	return slices.Contains(spec.Tags, File)
}

func (spec *SecretSpec) IsVar() bool {
	return slices.Contains(spec.Tags, Var)
}

func (spec *SecretSpec) IsLiteral() bool {
	return slices.Contains(spec.Tags, Literal)
}

// SecretsMap maps environment variable names or aliases to their SecretSpec.
type SecretsMap map[string]SecretSpec

// FileConfig represents a single file to be created with secrets.
type FileConfig struct {
	Path        string      `yaml:"path"`
	Format      string      `yaml:"format"`      // "template", "yaml", "dotenv", "json", etc.
	Template    string      `yaml:"template"`    // Custom template content
	Secrets     interface{} `yaml:"secrets"`     // Will be parsed as SecretsMap or map[string]SecretsMap
	Overwrite   bool        `yaml:"overwrite"`   // Whether to overwrite existing files
	Permissions os.FileMode `yaml:"permissions"` // File permissions (e.g., 0644)

	// secretsNode stores the raw YAML node to preserve tags during parsing
	secretsNode *yaml.Node
}

// Validate checks that the FileConfig has all required fields.
func (fileConfig *FileConfig) Validate() error {
	if fileConfig.Path == "" {
		return fmt.Errorf("file config is missing required 'path' field")
	}

	// Further validation is performed in secret_file.go's SecretFile.validate()
	// and validateSecretsAgainstSpecs() during processing

	return nil
}

// ParsedConfig holds the parsed secrets.yml content: environment variable
// secrets and file-based secret configurations.
type ParsedConfig struct {
	EnvSecrets SecretsMap
	Files      []FileConfig
}

func (config *ParsedConfig) HasEnvSecrets() bool {
	return len(config.EnvSecrets) > 0
}

func (config *ParsedConfig) HasFileSecrets() bool {
	return len(config.Files) > 0
}

func (config *ParsedConfig) FileSecrets() SecretsMap {
	fileSecrets := make(SecretsMap)
	for _, fileConfig := range config.Files {
		maps.Copy(fileSecrets, fileConfig.Secrets.(SecretsMap))
	}
	return fileSecrets
}
