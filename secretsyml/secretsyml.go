// Package secretsyml provides functions for parsing a string or file
// in secrets.yml format.
package secretsyml

import (
	"gopkg.in/yaml.v1"
	"io/ioutil"
	"regexp"
	"strings"
)

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
	r, _ := regexp.Compile("(var|file|str)")
	tags := r.FindAllString(tag, -1)
	if len(tags) == 0 {
		return false
	}
	for _, t := range tags {
		switch t {
		case "str":
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

// tagInSlice determines whether a YamlTag is in a list of YamlTag
func tagInSlice(a YamlTag, list []YamlTag) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}
