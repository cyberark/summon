// Package secretsyml provides functions for parsing a string or file
// in secrets.yml format.
package secretsyml

import (
	"fmt"
	"gopkg.in/yaml.v1"
	"io/ioutil"
	"regexp"
	"strconv"
)

var COMMON_SECTIONS = []string{"common", "default"}

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

type SecretSpec struct {
	Tags []YamlTag
	Path string
}

func (s *SecretSpec) IsFile() bool {
	return tagInSlice(File, s.Tags)
}

func (s *SecretSpec) IsVar() bool {
	return tagInSlice(Var, s.Tags)
}

func (s *SecretSpec) IsLiteral() bool {
	return tagInSlice(Literal, s.Tags)
}

type SecretsMap map[string]SecretSpec

func (spec *SecretSpec) SetYAML(tag string, value interface{}) bool {
	r, _ := regexp.Compile("(var|file|str|int|bool)")
	tags := r.FindAllString(tag, -1)
	if len(tags) == 0 {
		return false
	}
	for _, t := range tags {
		switch t {
		case "str", "int", "bool":
			spec.Tags = append(spec.Tags, Literal)
		case "file":
			spec.Tags = append(spec.Tags, File)
		case "var":
			spec.Tags = append(spec.Tags, Var)
		default:
			return false
		}
	}

	if s, ok := value.(string); ok {
		spec.Path = s
	} else if s, ok := value.(int); ok {
		spec.Path = strconv.Itoa(s)
	} else if s, ok := value.(bool); ok {
		spec.Path = strconv.FormatBool(s)
	} else {
		return false
	}
	return true
}

// ParseFromString parses a string in secrets.yml format to a map.
func ParseFromString(content, env string, subs map[string]string) (SecretsMap, error) {
	return parse(content, env, subs)
}

// ParseFromFile parses a file in secrets.yml format to a map.
func ParseFromFile(filepath, env string, subs map[string]string) (SecretsMap, error) {
	data, err := ioutil.ReadFile(filepath)
	if err != nil {
		return nil, err
	}
	return parse(string(data), env, subs)
}

// Wrapper for parsing yaml contents
func parse(ymlContent, env string, subs map[string]string) (SecretsMap, error) {
	if env == "" {
		return parseRegular(ymlContent, subs)
	}

	return parseEnvironment(ymlContent, env, subs)
}

// Parse secrets yaml that has environment sections
func parseEnvironment(ymlContent, env string, subs map[string]string) (SecretsMap, error) {
	out := make(map[string]map[string]SecretSpec)

	if err := yaml.Unmarshal([]byte(ymlContent), &out); err != nil {
		return nil, err
	}

	if _, ok := out[env]; !ok {
		return nil, fmt.Errorf("No such environment '%v' found in secrets file", env)
	}

	secretsMap := make(SecretsMap)

	for i, spec := range out[env] {
		err := spec.applySubstitutions(subs)
		if err != nil {
			return nil, err
		}

		secretsMap[i] = spec
	}

	// parse and merge optional 'common/default' section with secretsMap
	for _, section := range COMMON_SECTIONS {
		if _, ok := out[section]; ok {
			return parseAndMergeCommon(out[section], secretsMap, subs)
		}
	}

	return secretsMap, nil
}

func parseAndMergeCommon(out, secretsMap SecretsMap, subs map[string]string) (SecretsMap, error) {
	for i, spec := range out {
		// Skip any env vars that already exist in primary secrets map
		if _, ok := secretsMap[i]; ok {
			continue
		}

		if err := spec.applySubstitutions(subs); err != nil {
			return nil, err
		}

		secretsMap[i] = spec
	}

	return secretsMap, nil
}

// Parse a secrets yaml that has no environment sections
func parseRegular(ymlContent string, subs map[string]string) (SecretsMap, error) {
	out := make(map[string]SecretSpec)

	if err := yaml.Unmarshal([]byte(ymlContent), &out); err != nil {
		return nil, err
	}

	for i, spec := range out {
		err := spec.applySubstitutions(subs)
		if err != nil {
			return nil, err
		}

		out[i] = spec
	}

	return out, nil
}

func (spec *SecretSpec) applySubstitutions(subs map[string]string) error {
	VAR_REGEX := regexp.MustCompile(`\$(\$|\w+)`)
	var substitutionError error

	subFunc := func(variable string) string {
		variable = variable[1:]
		if variable == "$" {
			return "$"
		}
		text, ok := subs[variable]
		if ok {
			return text
		} else {
			substitutionError = fmt.Errorf("Variable %v not declared!", variable)
			return ""
		}
	}

	spec.Path = VAR_REGEX.ReplaceAllStringFunc(spec.Path, subFunc)
	return substitutionError
}

// tagInSlice determines whether a YamlTag is in a list of YamlTag
func tagInSlice(a YamlTag, list []YamlTag) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}
