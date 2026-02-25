package secretsyml

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestApplySubstitutions(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		subs     map[string]string
		expected string
		errMsg   string
	}{
		{
			name:     "Nil subs returns path unchanged",
			path:     "$env/secret",
			subs:     nil,
			expected: "$env/secret",
		},
		{
			name:     "Known variable is replaced",
			path:     "$env/secret",
			subs:     map[string]string{"env": "prod"},
			expected: "prod/secret",
		},
		{
			name:     "Escaped dollar sign",
			path:     "FOO$$BAR",
			subs:     map[string]string{},
			expected: "FOO$BAR",
		},
		{
			name:   "Undeclared variable returns error",
			path:   "$missing/secret",
			subs:   map[string]string{},
			errMsg: "variable missing not declared",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := SecretSpec{Path: tt.path}
			err := spec.applySubstitutions(tt.subs)
			if tt.errMsg != "" {
				assert.EqualError(t, err, tt.errMsg)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, spec.Path)
			}
		})
	}
}

func TestApplySubstitutionsToMap(t *testing.T) {
	t.Run("Substitution error propagates", func(t *testing.T) {
		secrets := SecretsMap{
			"VAR": {Path: "$undefined/path", Tags: []YamlTag{Var}},
		}
		_, err := applySubstitutionsToMap(secrets, map[string]string{})
		assert.EqualError(t, err, "variable undefined not declared")
	})

	t.Run("Successful substitution", func(t *testing.T) {
		secrets := SecretsMap{
			"VAR": {Path: "$env/path", Tags: []YamlTag{Var}},
		}
		result, err := applySubstitutionsToMap(secrets, map[string]string{"env": "prod"})
		assert.NoError(t, err)
		assert.Equal(t, "prod/path", result["VAR"].Path)
	})
}
