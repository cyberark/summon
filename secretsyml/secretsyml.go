// Package secretsyml provides functions for parsing a string or file
// in secrets.yml format.
package secretsyml

import (
	"fmt"
	"io/ioutil"
	"regexp"
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
	return tagInSlice(File, spec.Tags)
}

func (spec *SecretSpec) IsVar() bool {
	return tagInSlice(Var, spec.Tags)
}

func (spec *SecretSpec) IsLiteral() bool {
	return tagInSlice(Literal, spec.Tags)
}

type SecretsMap map[string]SecretSpec

func (spec *SecretSpec) SetYAML(tag string, value interface{}) error {
	r, _ := regexp.Compile("(var|file|str|int|bool|float|" + defaultValueRegex.String() + ")")
	tags := r.FindAllString(tag, -1)
	if len(tags) == 0 {
		spec.Tags = append(spec.Tags, Literal)
	}

	for _, t := range tags {
		switch {
		case t == "bool":
			fallthrough
		case t == "float":
			fallthrough
		case t == "int":
			fallthrough
		case t == "str":
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
			return fmt.Errorf("unknown tag type found!")
		}
	}

	if s, ok := value.(int); ok {
		spec.Path = strconv.Itoa(s)
	} else if s, ok := value.(bool); ok {
		spec.Path = strconv.FormatBool(s)
	} else if s, ok := value.(float64); ok {
		spec.Path = strconv.FormatFloat(s, 'f', -1, 64)
	} else if s, ok := value.(string); ok {
		spec.Path = s
	} else {
		return fmt.Errorf("unable to convert value to a known type!")
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
	out := make(map[string]SecretsMap)

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
	out := make(SecretsMap)

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
			substitutionError = fmt.Errorf("variable %v not declared", variable)
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
