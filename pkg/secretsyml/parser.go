package secretsyml

import (
	"fmt"
	"maps"
	"os"
	"regexp"
	"strconv"

	"gopkg.in/yaml.v3"
)

// Compiled regexes for YAML tag parsing.
var (
	defaultValueRegex = regexp.MustCompile(`default='(?P<defaultValue>.*)'`)
	tagRegex          = regexp.MustCompile("(var|file|str|int|bool|float|" + defaultValueRegex.String() + ")")
)

// ParseFromString parses a secrets.yml string into a ParsedConfig.
func ParseFromString(content, env string, subs map[string]string) (*ParsedConfig, error) {
	return parseConfig(content, env, subs)
}

// ParseFromFile reads and parses a secrets.yml file into a ParsedConfig.
func ParseFromFile(filepath, env string, subs map[string]string) (*ParsedConfig, error) {
	data, err := os.ReadFile(filepath)
	if err != nil {
		return nil, err
	}
	return parseConfig(string(data), env, subs)
}

// parseConfig parses a YAML configuration that may contain both environment
// variable secrets and file-based secrets (summon.files section).
func parseConfig(ymlContent, env string, subs map[string]string) (*ParsedConfig, error) {
	// Parse as yaml.Node to preserve tags
	var rootNode yaml.Node
	if err := yaml.Unmarshal([]byte(ymlContent), &rootNode); err != nil {
		return nil, err
	}

	config := &ParsedConfig{
		EnvSecrets: SecretsMap{},
		Files:      []FileConfig{},
	}

	// The root node is a Document node, get the actual content
	if len(rootNode.Content) == 0 {
		return config, nil
	}
	contentNode := rootNode.Content[0]

	if contentNode.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("invalid YAML structure: expected mapping")
	}

	// Process the mapping to separate files from env secrets
	envSecretsNode := yaml.Node{Kind: yaml.MappingNode}

	for i := 0; i < len(contentNode.Content); i += 2 {
		keyNode := contentNode.Content[i]
		valueNode := contentNode.Content[i+1]

		if keyNode.Value == "summon.files" {
			// Process files section
			if err := parseFilesSectionFromNode(valueNode, &config.Files, env, subs); err != nil {
				return nil, err
			}
		} else {
			// Add to env secrets node
			envSecretsNode.Content = append(envSecretsNode.Content, keyNode, valueNode)
		}
	}

	// Parse environment variable secrets if any exist
	if len(envSecretsNode.Content) > 0 {
		var err error
		config.EnvSecrets, err = parseEnvSecretsFromNode(&envSecretsNode, env, subs)
		if err != nil {
			return nil, err
		}
	}

	return config, nil
}

// --- YAML unmarshaling ---

// UnmarshalYAML preserves the raw YAML node for secrets so tags aren't lost.
func (fc *FileConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type rawFileConfig struct {
		Path        string      `yaml:"path"`
		Format      string      `yaml:"format"`
		Template    string      `yaml:"template"`
		Secrets     yaml.Node   `yaml:"secrets"`
		Overwrite   bool        `yaml:"overwrite"`
		Permissions os.FileMode `yaml:"permissions"`
	}

	var raw rawFileConfig
	if err := unmarshal(&raw); err != nil {
		return err
	}

	fc.Path = raw.Path
	fc.Format = raw.Format
	fc.Template = raw.Template
	fc.Overwrite = raw.Overwrite
	fc.Permissions = raw.Permissions

	if raw.Secrets.Kind != 0 {
		fc.secretsNode = &raw.Secrets
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
		err := spec.setYAML(v.Tag, v.Value)
		if err != nil {
			return err
		}

		(*secretMap)[k] = spec
	}

	return nil
}

// setYAML parses a YAML tag string and value into the SecretSpec's fields.
func (spec *SecretSpec) setYAML(tag string, value interface{}) error {
	tags := tagRegex.FindAllString(tag, -1)
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

// --- Node-level parsing helpers ---

func parseEnvSecretsFromNode(node *yaml.Node, env string, subs map[string]string) (SecretsMap, error) {
	if env == "" {
		return parseSimpleSecretsFromNode(node, subs)
	}
	if isEnvironmentBasedNode(node) {
		return parseEnvironmentBasedSecretsFromNode(node, env, subs, "secrets file")
	}
	return nil, fmt.Errorf("No such environment '%s' found in secrets file", env)
}

// parseFilesSectionFromNode parses the summon.files section from a yaml.Node.
func parseFilesSectionFromNode(node *yaml.Node, files *[]FileConfig, env string, subs map[string]string) error {
	if node.Kind != yaml.SequenceNode {
		return fmt.Errorf("summon.files must be a sequence/array")
	}

	for _, fileNode := range node.Content {
		var fc FileConfig

		// Decode will trigger UnmarshalYAML which preserves the secrets node
		if err := fileNode.Decode(&fc); err != nil {
			return fmt.Errorf("failed to decode file config: %w", err)
		}

		if err := processFileConfigWithNode(&fc, env, subs); err != nil {
			return fmt.Errorf("failed to process file config: %w", err)
		}

		*files = append(*files, fc)
	}

	return nil
}

func processFileConfigWithNode(fc *FileConfig, env string, subs map[string]string) error {
	if fc.Permissions == 0 {
		fc.Permissions = 0600
	}
	if fc.Format == "" {
		fc.Format = "template"
	}
	if fc.secretsNode == nil {
		return fmt.Errorf("no secrets defined")
	}

	var secretsMap SecretsMap
	var err error
	if isEnvironmentBasedNode(fc.secretsNode) {
		secretsMap, err = parseEnvironmentBasedSecretsFromNode(fc.secretsNode, env, subs, "file config")
	} else {
		secretsMap, err = parseSimpleSecretsFromNode(fc.secretsNode, subs)
	}
	if err != nil {
		return err
	}
	fc.Secrets = secretsMap
	return nil
}

// isEnvironmentBasedNode returns true if all top-level values are mappings
// (indicating environment sections like "production:", "staging:", etc.).
func isEnvironmentBasedNode(node *yaml.Node) bool {
	if node.Kind != yaml.MappingNode || len(node.Content) == 0 {
		return false
	}
	for i := 1; i < len(node.Content); i += 2 {
		if node.Content[i].Kind != yaml.MappingNode {
			return false
		}
	}
	return true
}

// parseSimpleSecretsFromNode processes a mapping node's key-value pairs
// directly to preserve YAML tags (e.g. !var, !file).
func parseSimpleSecretsFromNode(node *yaml.Node, subs map[string]string) (SecretsMap, error) {
	if node.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("expected mapping node for secrets, got kind %d", node.Kind)
	}

	secretsMap := make(SecretsMap, len(node.Content)/2)
	for i := 0; i < len(node.Content); i += 2 {
		key := node.Content[i].Value
		val := node.Content[i+1]

		var spec SecretSpec
		if err := spec.setYAML(val.Tag, val.Value); err != nil {
			return nil, fmt.Errorf("failed to parse secret %q: %w", key, err)
		}
		secretsMap[key] = spec
	}
	return applySubstitutionsToMap(secretsMap, subs)
}

// parseEnvironmentBasedSecretsFromNode parses environment-based secrets from a yaml.Node.
func parseEnvironmentBasedSecretsFromNode(node *yaml.Node, env string, subs map[string]string, context string) (SecretsMap, error) {
	envSecrets := make(map[string]SecretsMap)
	if err := node.Decode(&envSecrets); err != nil {
		return nil, fmt.Errorf("failed to parse secrets for %s: %w", context, err)
	}

	if env == "" {
		return nil, fmt.Errorf("environment sections exist in %s but no environment specified", context)
	}

	targetSecrets, ok := envSecrets[env]
	if !ok {
		return nil, fmt.Errorf("No such environment '%s' found in %s", env, context)
	}

	targetSecrets, err := applySubstitutionsToMap(targetSecrets, subs)
	if err != nil {
		return nil, err
	}

	// Merge common/default sections
	return mergeCommonSections(targetSecrets, envSecrets, subs)
}

// mergeCommonSections returns a new SecretsMap with common/default entries
// merged in; target entries take precedence over common ones.
func mergeCommonSections(target SecretsMap, allSections map[string]SecretsMap, subs map[string]string) (SecretsMap, error) {
	merged := make(SecretsMap, len(target))
	maps.Copy(merged, target)

	for _, commonKey := range commonSections {
		if commonSecrets, hasCommon := allSections[commonKey]; hasCommon {
			commonSecrets, err := applySubstitutionsToMap(commonSecrets, subs)
			if err != nil {
				return nil, err
			}

			for k, v := range commonSecrets {
				if _, exists := merged[k]; !exists {
					merged[k] = v
				}
			}
			break
		}
	}
	return merged, nil
}
