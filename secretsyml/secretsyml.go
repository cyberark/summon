package secretsyml

import (
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"strings"
)

func ParseFile(filepath string) (map[string]string, error) {
	data, err := ioutil.ReadFile(filepath)
	if err != nil {
		return nil, err
	}
	return parse(string(data))
}

func parse(ymlContent string) (map[string]string, error) {
	// Replace our custom !file tag with one that will be serialized
	// The result is the format <KEY>:file <VALUE>
	taggedData := strings.Replace(ymlContent, "!file", "!!set file", -1)
	out := make(map[string]string)

	err := yaml.Unmarshal([]byte(taggedData), &out)

	if err != nil {
		return nil, err
	}

	return out, nil
}
