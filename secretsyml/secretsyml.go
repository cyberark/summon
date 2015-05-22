// Package secretsyml provides functions for parsing a string or file
// in secrets.yml format.
package secretsyml

import (
	"gopkg.in/yaml.v1"
	"io/ioutil"
	"strings"
)

type secretKind uint8

const (
	SecretFile    secretKind = iota
	SecretVar     secretKind = iota
	SecretLiteral secretKind = iota
)

func (k secretKind) String() string {
	switch k {
	case SecretFile:
		return "File"
	case SecretVar:
		return "Var"
	case SecretLiteral:
		return "Literal"
	default:
		panic("unreachable!")
	}
}

type SecretSpec struct {
	Kind secretKind
	Path string
}

func (s *SecretSpec) IsFile() bool {
	return s.Kind == SecretFile
}

func (s *SecretSpec) IsVar() bool {
	return s.Kind == SecretVar
}

func (s *SecretSpec) IsLiteral() bool {
	return s.Kind == SecretLiteral
}

type SecretsMap map[string]SecretSpec

func (spec *SecretSpec) SetYAML(tag string, value interface{}) bool {
	var kind secretKind
	switch tag {
	case "!!str":
		kind = SecretLiteral
	case "!file":
		kind = SecretFile
	case "!var":
		kind = SecretVar
	default:
		return false
	}
	spec.Kind = kind
	if s, ok := value.(string); ok {
		spec.Path = s
	} else {
		return false
	}
	return true
}

// ParseFromString parses a string in secrets.yml format to a map.
func ParseFromString(content string, subs map[string]string) (SecretsMap, error) {
	return parse(content, subs)
}

// ParseFromFile parses a file in secrets.yml format to a map.
func ParseFromFile(filepath string, subs map[string]string) (SecretsMap, error) {
	data, err := ioutil.ReadFile(filepath)
	if err != nil {
		return nil, err
	}
	return parse(string(data), subs)
}

func parse(ymlContent string, subs map[string]string) (SecretsMap, error) {
	applySubstitutions(&ymlContent, subs)
	out := make(map[string]SecretSpec)

	err := yaml.Unmarshal([]byte(ymlContent), &out)

	if err != nil {
		return nil, err
	}

	return out, nil
}

func applySubstitutions(ymlContent *string, subs map[string]string) {
	for key, val := range subs {
		*ymlContent = strings.Replace(*ymlContent, key, val, -1)
	}
}
