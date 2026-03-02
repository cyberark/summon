package pushtofile

import (
	"bytes"
	"os"
	"testing"

	filetemplates "github.com/cyberark/summon/pkg/file_templates"
	"github.com/stretchr/testify/assert"
)

type pushToWriterTestCase struct {
	description string
	template    string
	secrets     []*filetemplates.Secret
	assert      func(*testing.T, string, error)
}

func (tc pushToWriterTestCase) Run(t *testing.T) {
	t.Run(tc.description, func(t *testing.T) {
		buf := new(bytes.Buffer)
		err := pushToWriter(
			buf,
			"filepath",
			tc.template,
			tc.secrets,
		)
		tc.assert(t, buf.String(), err)
	})
}

func assertGoodOutput(expected string) func(*testing.T, string, error) {
	return func(t *testing.T, actual string, err error) {
		if !assert.NoError(t, err) {
			return
		}

		assert.Equal(
			t,
			expected,
			actual,
		)
	}
}

var writeToFileTestCases = []pushToWriterTestCase{
	{
		description: "happy path",
		template:    `{{secret "alias"}}`,
		secrets:     []*filetemplates.Secret{{Alias: "alias", Value: "secret value"}},
		assert:      assertGoodOutput("secret value"),
	},
	{
		description: "undefined secret",
		template:    `{{secret "x"}}`,
		secrets:     []*filetemplates.Secret{{Alias: "some alias", Value: "secret value"}},
		assert: func(t *testing.T, s string, err error) {
			assert.Error(t, err)
			assert.Contains(t, err.Error(), `secret alias "x" not present in specified secrets for file`)
		},
	},
	{
		// Conversions defined in Go source:
		// https://cs.opensource.google/go/go/+/refs/tags/go1.17.2:src/text/template/funcs.go;l=608
		description: "confirm use of built-in html escape template function",
		template:    `{{secret "alias" | html}}`,
		secrets:     []*filetemplates.Secret{{Alias: "alias", Value: "\" ' & < > \000"}},
		assert:      assertGoodOutput("&#34; &#39; &amp; &lt; &gt; \uFFFD"),
	},
	{
		description: "base64 encoding",
		template:    `{{secret "alias" | b64enc}}`,
		secrets:     []*filetemplates.Secret{{Alias: "alias", Value: "secret value"}},
		assert:      assertGoodOutput("c2VjcmV0IHZhbHVl"),
	},
	{
		description: "base64 decoding",
		template:    `{{secret "alias" | b64dec}}`,
		secrets:     []*filetemplates.Secret{{Alias: "alias", Value: "c2VjcmV0IHZhbHVl"}},
		assert:      assertGoodOutput("secret value"),
	},
	{
		description: "base64 decoding invalid input",
		template:    `{{secret "alias" | b64dec}}`,
		secrets:     []*filetemplates.Secret{{Alias: "alias", Value: "c2VjcmV0IHZhbHVl_invalid"}},
		assert: func(t *testing.T, s string, err error) {
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "value could not be base64 decoded")
			// Ensure the error doesn't contain the actual secret
			assert.NotContains(t, err.Error(), "c2VjcmV0IHZhbHVl_invalid")
		},
	},
	{
		description: "iterate over secret key-value pairs",
		template: `{{- range $index, $secret := .SecretsArray -}}
{{- if $index }}
{{ end }}
{{- $secret.Alias }}: {{ $secret.Value }}
{{- end -}}`,
		secrets: []*filetemplates.Secret{
			{Alias: "environment", Value: "prod"},
			{Alias: "url", Value: "https://example.com"},
			{Alias: "username", Value: "example-user"},
			{Alias: "password", Value: "example-pass"},
		},
		assert: assertGoodOutput(`environment: prod
password: example-pass
url: https://example.com
username: example-user`),
	},
	{
		description: "nested templates",
		template: `{{- define "contents" -}}
Alias : {{ .Alias }}
Value : {{ .Value }}
{{ end }}
{{- define "parent" -}}
Nested Template
{{ template "contents" . -}}
===============
{{ end }}
{{- range $index, $secret := .SecretsArray -}}
{{ template "parent" . }}
{{- end -}}`,
		secrets: []*filetemplates.Secret{
			{Alias: "environment", Value: "prod"},
			{Alias: "url", Value: "https://example.com"},
			{Alias: "username", Value: "example-user"},
			{Alias: "password", Value: "example-pass"},
		},
		assert: assertGoodOutput(`Nested Template
Alias : environment
Value : prod
===============
Nested Template
Alias : password
Value : example-pass
===============
Nested Template
Alias : url
Value : https://example.com
===============
Nested Template
Alias : username
Value : example-user
===============
`),
	},
	{
		// .SecretsArray is sorted lexicographically by alias regardless of input order.
		description: "SecretsArray sorted iteration",
		template:    "{{range .SecretsArray}}{{.Alias}}: {{.Value}}\n{{end}}",
		secrets: []*filetemplates.Secret{
			{Alias: "z", Value: "1"},
			{Alias: "a", Value: "2"},
			{Alias: "m", Value: "3"},
		},
		assert: assertGoodOutput("a: 2\nm: 3\nz: 1\n"),
	},
	{
		// Smoke test to ensure summon template features do not break existing text/template blocks.
		description: "nested define and template",
		template:    `{{define "sub"}}inner{{end}}{{template "sub" .}}`,
		secrets:     []*filetemplates.Secret{{Alias: "alias", Value: "value"}},
		assert:      assertGoodOutput("inner"),
	},
	// The following cases assert that native text/template constructs work
	// as-is inside summon templates. Summon wraps text/template and adds a
	// small FuncMap of custom template functions, but must not restrict
	// any built-in language features that users rely on.
	{
		description: "native text/template: if/else conditional on secret value",
		template:    `{{if secret "flag"}}enabled{{else}}disabled{{end}}`,
		secrets:     []*filetemplates.Secret{{Alias: "flag", Value: "true"}},
		assert:      assertGoodOutput("enabled"),
	},
	{
		description: "native text/template: with block re-scopes dot",
		template:    `{{with .SecretsMap.key}}{{.Value}}{{end}}`,
		secrets:     []*filetemplates.Secret{{Alias: "key", Value: "scoped-value"}},
		assert:      assertGoodOutput("scoped-value"),
	},
	{
		description: "native text/template: printf formats a secret value",
		template:    `{{printf "host=%s port=%s" (secret "host") (secret "port")}}`,
		secrets: []*filetemplates.Secret{
			{Alias: "host", Value: "db.internal"},
			{Alias: "port", Value: "5432"},
		},
		assert: assertGoodOutput("host=db.internal port=5432"),
	},
	{
		description: "native text/template: variable assignment and reuse",
		template:    `{{$u := secret "user"}}{{$p := secret "pass"}}{{$u}}:{{$p}}`,
		secrets: []*filetemplates.Secret{
			{Alias: "user", Value: "admin"},
			{Alias: "pass", Value: "s3cr3t"},
		},
		assert: assertGoodOutput("admin:s3cr3t"),
	},
}

func Test_pushToWriter(t *testing.T) {
	for _, tc := range writeToFileTestCases {
		tc.Run(t)
	}
}

func Test_dirPermsForFilePerms(t *testing.T) {
	tests := []struct {
		description string
		filePerms   os.FileMode
		expected    os.FileMode
	}{
		{
			description: "owner-only read/write returns 0700",
			filePerms:   0600,
			expected:    0700,
		},
		{
			description: "owner-only read returns 0700",
			filePerms:   0400,
			expected:    0700,
		},
		{
			description: "group read returns 0750",
			filePerms:   0640,
			expected:    0750,
		},
		{
			description: "group read/write returns 0750",
			filePerms:   0660,
			expected:    0750,
		},
		{
			description: "other read returns 0755",
			filePerms:   0644,
			expected:    0755,
		},
		{
			description: "world read/write returns 0755",
			filePerms:   0666,
			expected:    0755,
		},
		{
			description: "other execute only returns 0755",
			filePerms:   0601,
			expected:    0755,
		},
		{
			description: "group execute only returns 0750",
			filePerms:   0610,
			expected:    0750,
		},
		{
			description: "no permissions returns 0700",
			filePerms:   0000,
			expected:    0700,
		},
	}

	for _, tc := range tests {
		t.Run(tc.description, func(t *testing.T) {
			actual := dirPermsForFilePerms(tc.filePerms)
			assert.Equal(t, tc.expected, actual)
		})
	}
}
