package secretsyml

import (
	"fmt"
	"regexp"
)

// varSubstRegex matches $variable or $$ (escaped dollar sign) in secret paths.
var varSubstRegex = regexp.MustCompile(`\$(\$|\w+)`)

// applySubstitutions replaces $variable references in the spec's Path.
func (spec *SecretSpec) applySubstitutions(subs map[string]string) error {
	if subs == nil {
		return nil
	}

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

	spec.Path = varSubstRegex.ReplaceAllStringFunc(spec.Path, subFunc)
	return substitutionError
}

// applySubstitutionsToMap applies variable substitutions to all secrets in a map.
func applySubstitutionsToMap(secretsMap SecretsMap, subs map[string]string) (SecretsMap, error) {
	for key, spec := range secretsMap {
		if err := spec.applySubstitutions(subs); err != nil {
			return nil, err
		}
		secretsMap[key] = spec
	}
	return secretsMap, nil
}
