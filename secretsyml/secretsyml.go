// Package secretsyml provides functions for parsing a string or file
// in secrets.yml format.
package secretsyml

import (
	"gopkg.in/yaml.v1"
	"io/ioutil"
	"strings"
)

type SecretSpec struct {
	IsFile bool
	Path   string
}

func (spec *SecretSpec) SetYAML(tag string, value interface{}) bool {
	spec.IsFile = (tag == "!file")
	if s, ok := value.(string); ok {
		spec.Path = s
	} else {
		return false
	}
	return true
}

// ParseFromString parses a string in secrets.yml format to a map.
func ParseFromString(content string, subs map[string]string) (map[string]SecretSpec, error) {
	return parse(content, subs)
}

// ParseFromFile parses a file in secrets.yml format to a map.
func ParseFromFile(filepath string, subs map[string]string) (map[string]SecretSpec, error) {
	data, err := ioutil.ReadFile(filepath)
	if err != nil {
		return nil, err
	}
	return parse(string(data), subs)
}

func parse(ymlContent string, subs map[string]string) (map[string]SecretSpec, error) {
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
		println(key, val)
		*ymlContent = strings.Replace(*ymlContent, key, val, -1)
	}
}
