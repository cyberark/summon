// Package secretsyml provides functions for parsing a string or file
// in secrets.yml format.
package secretsyml

import (
	"fmt"
	"os"
	"regexp"
	"slices"
	"strconv"

	"gopkg.in/yaml.v3"
)

var COMMON_SECTIONS = []string{"common", "default"}

type YamlTag uint8

const (
	File YamlTag = iota
	Var
	Literal
)

var defaultValueRegex = regexp.MustCompile(`default='(?P<defaultValue>.*)'`)

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

type SecretSpec struct {
	Tags         []YamlTag
	Path         string
	DefaultValue string
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

type SecretsMap map[string]SecretSpec

// FileConfig represents a single file to be created with secrets
type FileConfig struct {
	Path        string      `yaml:"path"`
	Format      string      `yaml:"format"`      // "template", "yaml", "dotenv", "json", etc.
	Template    string      `yaml:"template"`    // Custom template content
	Secrets     interface{} `yaml:"secrets"`     // Can be SecretsMap or map[string]SecretsMap for environments
	Overwrite   bool        `yaml:"overwrite"`   // Whether to overwrite existing files
	Permissions os.FileMode `yaml:"permissions"` // File permissions (e.g., 0644)
}

// ParsedConfig represents the result of parsing a secrets.yml file
// containing both environment variable secrets and file-based secrets
type ParsedConfig struct {
	// EnvSecrets contains the secrets to be injected as environment variables
	EnvSecrets SecretsMap

	// Files contains the file configurations with their associated secrets
	Files []FileConfig
}

func (spec *SecretSpec) SetYAML(tag string, value interface{}) error {
	r, _ := regexp.Compile("(var|file|str|int|bool|float|" + defaultValueRegex.String() + ")")
	tags := r.FindAllString(tag, -1)
	if len(tags) == 0 {
		spec.Tags = append(spec.Tags, Literal)
	}

	for _, t := range tags {
		switch {
		case t == "bool", t == "float", t == "int", t == "str":
			spec.Tags = append(spec.Tags, Literal)
		case t == "file":
			spec.Tags = append(spec.Tags, File)
		case t == "var":
			spec.Tags = append(spec.Tags, Var)
		case defaultValueRegex.MatchString(t):
			match := defaultValueRegex.FindStringSubmatch(t)
			spec.DefaultValue = match[1]

			if len(tags) == 1 {
				spec.Tags = append(spec.Tags, Literal)
			}
		default:
			return fmt.Errorf("unknown tag type: %s", t)
		}
	}

	switch v := value.(type) {
	case int:
		spec.Path = strconv.Itoa(v)
	case bool:
		spec.Path = strconv.FormatBool(v)
	case float64:
		spec.Path = strconv.FormatFloat(v, 'f', -1, 64)
	case string:
		spec.Path = v
	default:
		return fmt.Errorf("unable to convert value to a known type")
	}

	return nil
}

func (secretMap *SecretsMap) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*secretMap = SecretsMap{}

	m := map[string]yaml.Node{}
	if err := unmarshal(&m); err != nil {
		return err
	}

	for k, v := range m {
		spec := SecretSpec{}
		err := spec.SetYAML(v.Tag, v.Value)
		if err != nil {
			return err
		}

		(*secretMap)[k] = spec
	}

	return nil
}

// ParseFromString parses a string in the new secrets.yml format that supports
// both environment variables and file-based secrets.
func ParseFromString(content, env string, subs map[string]string) (*ParsedConfig, error) {
	return parseConfig(content, env, subs)
}

// ParseFromFile parses a file in the new secrets.yml format that supports
// both environment variables and file-based secrets.
func ParseFromFile(filepath, env string, subs map[string]string) (*ParsedConfig, error) {
	data, err := os.ReadFile(filepath)
	if err != nil {
		return nil, err
	}
	return parseConfig(string(data), env, subs)
}

func (spec *SecretSpec) applySubstitutions(subs map[string]string) error {
	if subs == nil {
		return nil
	}

	VAR_REGEX := regexp.MustCompile(`\$(\$|\w+)`)
	var substitutionError error

	subFunc := func(variable string) string {
		variable = variable[1:]
		if variable == "$" {
			return "$"
		}
		if text, ok := subs[variable]; ok {
			return text
		}
		substitutionError = fmt.Errorf("variable %v not declared", variable)
		return ""
	}

	spec.Path = VAR_REGEX.ReplaceAllStringFunc(spec.Path, subFunc)
	return substitutionError
}

// parseConfig parses a YAML configuration that may contain both environment
// variable secrets and file-based secrets (summon.files section).
func parseConfig(ymlContent, env string, subs map[string]string) (*ParsedConfig, error) {
	// First, try to parse as a raw YAML to detect structure
	rawConfig := make(map[string]interface{})
	if err := yaml.Unmarshal([]byte(ymlContent), &rawConfig); err != nil {
		return nil, err
	}

	config := &ParsedConfig{
		EnvSecrets: SecretsMap{},
		Files:      []FileConfig{},
	}

	// Process file-based secrets if present
	if filesSection, hasFiles := rawConfig["summon.files"]; hasFiles {
		if err := parseFilesSection(filesSection, &config.Files, env, subs); err != nil {
			return nil, err
		}

		// Remove summon.files before parsing environment variables
		delete(rawConfig, "summon.files")
		ymlContent = remarshalConfig(rawConfig)
	}

	// Parse environment variable secrets (if any remain)
	if len(rawConfig) > 0 {
		var err error
		config.EnvSecrets, err = parseEnvSecrets(ymlContent, env, subs)
		if err != nil {
			return nil, err
		}
	}

	return config, nil
}

// parseEnvSecrets parses environment variable secrets with or without environment sections
func parseEnvSecrets(ymlContent, env string, subs map[string]string) (SecretsMap, error) {
	if env == "" {
		// Parse without environment sections
		envSecrets := SecretsMap{}
		if err := yaml.Unmarshal([]byte(ymlContent), &envSecrets); err != nil {
			return nil, err
		}
		return applySubstitutionsToMap(envSecrets, subs)
	}

	// Parse with environment sections
	out := make(map[string]SecretsMap)
	if err := yaml.Unmarshal([]byte(ymlContent), &out); err != nil {
		// Check if the error is due to there being no environment sections
		testMap := SecretsMap{}
		if testErr := yaml.Unmarshal([]byte(ymlContent), &testMap); testErr == nil {
			// If a regular parse is successful, then the error is due to the environment not existing
			return nil, fmt.Errorf("No such environment '%v' found in secrets file", env)
		}
		// Otherwise, return the YAML parsing's original error
		return nil, err
	}

	if _, ok := out[env]; !ok {
		return nil, fmt.Errorf("No such environment '%v' found in secrets file", env)
	}

	envSecrets, err := applySubstitutionsToMap(out[env], subs)
	if err != nil {
		return nil, err
	}

	// Merge optional 'common/default' section with envSecrets
	if err := mergeCommonSections(envSecrets, out, subs); err != nil {
		return nil, err
	}

	return envSecrets, nil
}

// processFileConfig processes a single FileConfig, parsing its secrets and applying substitutions
func processFileConfig(fc *FileConfig, env string, subs map[string]string) error {
	setFileDefaults(fc)

	if fc.Secrets == nil {
		return fmt.Errorf("file config for path '%s' has no secrets defined", fc.Path)
	}

	secretsMap, err := parseFileSecrets(fc.Secrets, env, subs, fc.Path)
	if err != nil {
		return err
	}

	fc.Secrets = secretsMap
	return nil
}

// setFileDefaults sets default values for file configuration
func setFileDefaults(fc *FileConfig) {
	if fc.Permissions == 0 {
		fc.Permissions = 0600
	}
	if fc.Format == "" {
		fc.Format = "template"
	}
}

// parseFileSecrets parses secrets for a file configuration, handling both
// simple and environment-based formats
func parseFileSecrets(secrets interface{}, env string, subs map[string]string, filePath string) (SecretsMap, error) {
	secretsBytes, err := yaml.Marshal(secrets)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal secrets for file '%s': %w", filePath, err)
	}

	// Check if secrets are environment-based
	if isEnvironmentBased(secretsBytes) {
		return parseEnvironmentBasedSecrets(secretsBytes, env, subs, filePath)
	}

	// Parse as simple secrets map
	return parseSimpleSecrets(secretsBytes, subs)
}

// isEnvironmentBased checks if the secrets structure is environment-based
// by verifying all top-level values are maps
func isEnvironmentBased(secretsBytes []byte) bool {
	var rawStructure map[string]interface{}
	if err := yaml.Unmarshal(secretsBytes, &rawStructure); err != nil {
		return false
	}

	for _, value := range rawStructure {
		if _, isMap := value.(map[string]interface{}); !isMap {
			return false
		}
	}
	return true
}

// parseSimpleSecrets parses a simple (non-environment-based) secrets map
func parseSimpleSecrets(secretsBytes []byte, subs map[string]string) (SecretsMap, error) {
	secretsMap := SecretsMap{}
	if err := yaml.Unmarshal(secretsBytes, &secretsMap); err != nil {
		return nil, err
	}

	return applySubstitutionsToMap(secretsMap, subs)
}

// parseEnvironmentBasedSecrets parses environment-based secrets and merges with common sections
func parseEnvironmentBasedSecrets(secretsBytes []byte, env string, subs map[string]string, filePath string) (SecretsMap, error) {
	envSecrets := make(map[string]SecretsMap)
	if err := yaml.Unmarshal(secretsBytes, &envSecrets); err != nil {
		return nil, fmt.Errorf("failed to parse secrets for file '%s': %w", filePath, err)
	}

	if env == "" {
		return nil, fmt.Errorf("file config for '%s' has environment sections but no environment specified", filePath)
	}

	if _, ok := envSecrets[env]; !ok {
		return nil, fmt.Errorf("No such environment '%s' found in file config for '%s'", env, filePath)
	}

	secretsMap, err := applySubstitutionsToMap(envSecrets[env], subs)
	if err != nil {
		return nil, err
	}

	// Merge with common/default section
	if err := mergeCommonSections(secretsMap, envSecrets, subs); err != nil {
		return nil, err
	}

	return secretsMap, nil
}

// applySubstitutionsToMap applies variable substitutions to all secrets in a map
func applySubstitutionsToMap(secretsMap SecretsMap, subs map[string]string) (SecretsMap, error) {
	for key, spec := range secretsMap {
		if err := spec.applySubstitutions(subs); err != nil {
			return nil, err
		}
		secretsMap[key] = spec
	}
	return secretsMap, nil
}

// mergeCommonSections merges common/default sections into the target secrets map
// from the allSections map (which contains all environment sections including common/default)
func mergeCommonSections(target SecretsMap, allSections map[string]SecretsMap, subs map[string]string) error {
	for _, section := range COMMON_SECTIONS {
		if commonSecrets, ok := allSections[section]; ok {
			for key, spec := range commonSecrets {
				if _, exists := target[key]; !exists {
					if err := spec.applySubstitutions(subs); err != nil {
						return err
					}
					target[key] = spec
				}
			}
			break
		}
	}
	return nil
}

// parseFilesSection parses the summon.files section and processes each file config
func parseFilesSection(filesSection interface{}, files *[]FileConfig, env string, subs map[string]string) error {
	filesBytes, err := yaml.Marshal(filesSection)
	if err != nil {
		return fmt.Errorf("failed to parse summon.files section: %w", err)
	}

	if err := yaml.Unmarshal(filesBytes, files); err != nil {
		return fmt.Errorf("failed to parse summon.files section: %w", err)
	}

	for i := range *files {
		if err := processFileConfig(&(*files)[i], env, subs); err != nil {
			return fmt.Errorf("failed to process file config at index %d: %w", i, err)
		}
	}

	return nil
}

// remarshalConfig re-marshals a config map back to YAML string
func remarshalConfig(config map[string]interface{}) string {
	if bytes, err := yaml.Marshal(config); err == nil {
		return string(bytes)
	}
	return ""
}
