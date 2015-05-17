// Package secretsyml provides functions for parsing a string or file
// in secrets.yml format.
package secretsyml

import (
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"strings"
)

// ParseFromString parses a string in secrets.yml format to a map.
func ParseFromString(content string, subs map[string]string) (map[string]string, error) {
	return parse(content, subs)
}

// ParseFromFile parses a file in secrets.yml format to a map.
func ParseFromFile(filepath string, subs map[string]string) (map[string]string, error) {
	data, err := ioutil.ReadFile(filepath)
	if err != nil {
		return nil, err
	}
	return parse(string(data), subs)
}

func parse(ymlContent string, subs map[string]string) (map[string]string, error) {
	// Replace our custom !file tag with one that will be serialized
	// The result is the format <KEY>:file <VALUE>
	taggedData := strings.Replace(ymlContent, "!file", "!!set file", -1)
	applySubstitutions(&taggedData, subs)
	out := make(map[string]string)

	err := yaml.Unmarshal([]byte(taggedData), &out)

	if err != nil {
		return nil, err
	}

	return out, nil
}

func applySubstitutions(ymlContent *string, subs map[string]string) {
	for key, val := range subs {
		println(key, val)
		ymlContent = strings.Replace(ymlContent, key, val, -1)
	}
}
