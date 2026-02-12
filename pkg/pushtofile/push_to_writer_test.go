package pushtofile

import (
	"bytes"
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
url: https://example.com
username: example-user
password: example-pass`),
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
Alias : url
Value : https://example.com
===============
Nested Template
Alias : username
Value : example-user
===============
Nested Template
Alias : password
Value : example-pass
===============
`),
	},
}

func Test_pushToWriter(t *testing.T) {
	for _, tc := range writeToFileTestCases {
		tc.Run(t)
	}
}
