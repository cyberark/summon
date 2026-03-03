package filetemplates

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"text/template"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderFile(t *testing.T) {
	tests := []struct {
		name     string
		template string
		tplData  TemplateData
		want     string
		wantErr  bool
	}{
		{
			name:     "simple template with secret value",
			template: `{{ .SecretsMap.secret1.Value }}`,
			tplData: TemplateData{
				SecretsMap: map[string]*Secret{
					"secret1": {Alias: "secret1", Value: "value1"},
				},
			},
			want:    "value1",
			wantErr: false,
		},
		{
			name:     "template using SecretsArray",
			template: `{{ range .SecretsArray }}{{ .Alias }}:{{ .Value }}{{ end }}`,
			tplData: TemplateData{
				SecretsArray: []*Secret{
					{Alias: "key1", Value: "val1"},
					{Alias: "key2", Value: "val2"},
				},
			},
			want:    "key1:val1key2:val2",
			wantErr: false,
		},
		{
			name:     "empty template",
			template: ``,
			tplData: TemplateData{
				SecretsMap: map[string]*Secret{},
			},
			want:    "",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tpl, err := template.New("test").Parse(tt.template)
			assert.NoError(t, err)

			buf, err := RenderFile(tpl, tt.tplData)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, buf.String())
			}
		})
	}
}

func TestGetTemplate(t *testing.T) {
	tests := []struct {
		name       string
		template   string
		secretsMap map[string]*Secret
		want       string
		wantErr    bool
		errMsg     string
	}{
		{
			name:     "secret function returns correct value",
			template: `{{ secret "my-secret" }}`,
			secretsMap: map[string]*Secret{
				"my-secret": {Alias: "my-secret", Value: "secret-value"},
			},
			want:    "secret-value",
			wantErr: false,
		},
		{
			name:     "secret function with multiple secrets",
			template: `{{ secret "username" }}:{{ secret "password" }}`,
			secretsMap: map[string]*Secret{
				"username": {Alias: "username", Value: "admin"},
				"password": {Alias: "password", Value: "pass123"},
			},
			want:    "admin:pass123",
			wantErr: false,
		},
		{
			name:     "secret function panics on missing alias",
			template: `{{ secret "missing" }}`,
			secretsMap: map[string]*Secret{
				"existing": {Alias: "existing", Value: "value"},
			},
			wantErr: true,
			errMsg:  `secret alias "missing" not present`,
		},
		{
			name:     "b64enc function encodes correctly",
			template: `{{ secret "secret" | b64enc }}`,
			secretsMap: map[string]*Secret{
				"secret": {Alias: "secret", Value: "hello world"},
			},
			want:    "aGVsbG8gd29ybGQ=",
			wantErr: false,
		},
		{
			name:     "b64dec function decodes correctly",
			template: `{{ secret "secret" | b64dec }}`,
			secretsMap: map[string]*Secret{
				"secret": {Alias: "secret", Value: "aGVsbG8gd29ybGQ="},
			},
			want:    "hello world",
			wantErr: false,
		},
		{
			name:     "b64dec function panics on invalid base64",
			template: `{{ secret "secret" | b64dec }}`,
			secretsMap: map[string]*Secret{
				"secret": {Alias: "secret", Value: "invalid-base64!!!"},
			},
			wantErr: true,
			errMsg:  "value could not be base64 decoded",
		},
		{
			name:     "chaining template functions",
			template: `{{ secret "secret" | b64enc | b64dec }}`,
			secretsMap: map[string]*Secret{
				"secret": {Alias: "secret", Value: "test"},
			},
			want:    "test",
			wantErr: false,
		},
		{
			name: "TOML: database config section with quoted string values",
			template: `[database]
host = "{{ secret "db_host" }}"
password = "{{ secret "db_pass" }}"`,
			secretsMap: map[string]*Secret{
				"db_host": {Alias: "db_host", Value: "prod.db.internal"},
				"db_pass": {Alias: "db_pass", Value: "s3cr3t!"},
			},
			want: `[database]
host = "prod.db.internal"
password = "s3cr3t!"`,
			wantErr: false,
		},
		{
			name:     "kubeconfig YAML: certificate-authority-data is base64-encoded with b64enc",
			template: `certificate-authority-data: {{ secret "ca_cert" | b64enc }}`,
			secretsMap: map[string]*Secret{
				"ca_cert": {Alias: "ca_cert", Value: "cert-data"},
			},
			// "cert-data" base64-encoded = "Y2VydC1kYXRh"
			want:    "certificate-authority-data: Y2VydC1kYXRh",
			wantErr: false,
		},
		{
			name: "AWS credentials: INI-style file with unquoted secret values",
			template: `[default]
aws_access_key_id = {{ secret "access_key" }}
aws_secret_access_key = {{ secret "secret_key" }}`,
			secretsMap: map[string]*Secret{
				"access_key": {Alias: "access_key", Value: "AKIAIOSFODNN7EXAMPLE"},
				"secret_key": {Alias: "secret_key", Value: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"},
			},
			want: `[default]
aws_access_key_id = AKIAIOSFODNN7EXAMPLE
aws_secret_access_key = wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY`,
			wantErr: false,
		},
		{
			name: "HCL: Terraform provider block with quoted string values",
			template: `provider "aws" {
  access_key = "{{ secret "access_key" }}"
  secret_key = "{{ secret "secret_key" }}"
  region     = "{{ secret "region" }}"
}`,
			secretsMap: map[string]*Secret{
				"access_key": {Alias: "access_key", Value: "AKIAIOSFODNN7EXAMPLE"},
				"secret_key": {Alias: "secret_key", Value: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"},
				"region":     {Alias: "region", Value: "us-east-1"},
			},
			want: `provider "aws" {
  access_key = "AKIAIOSFODNN7EXAMPLE"
  secret_key = "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
  region     = "us-east-1"
}`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tpl := GetTemplate("test", tt.secretsMap)
			parsedTpl, err := tpl.Parse(tt.template)
			assert.NoError(t, err)

			tplData := TemplateData{
				SecretsMap: tt.secretsMap,
			}

			buf, err := RenderFile(parsedTpl, tplData)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, buf.String())
			}
		})
	}
}

// assertValidXML wraps rendered output in a root element and parses it,
// confirming the output is well-formed XML.
func assertValidXML(t *testing.T, output string) {
	t.Helper()
	wrapped := fmt.Sprintf("<root>%s</root>", output)
	decoder := xml.NewDecoder(strings.NewReader(wrapped))
	for {
		_, err := decoder.Token()
		if err != nil && errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err, "rendered output is not well-formed XML:\n%s", output)
	}
}

func TestWebConfigXML(t *testing.T) {
	tests := []struct {
		description string
		template    string
		secretsMap  map[string]*Secret
		wantOutput  string
	}{
		{
			description: "safe value: no special characters",
			template:    `<add key="Password" value="{{ secret "db_pass" }}" />`,
			secretsMap: map[string]*Secret{
				"db_pass": {Alias: "db_pass", Value: "SafePassword123"},
			},
			wantOutput: `<add key="Password" value="SafePassword123" />`,
		},
		{
			description: "special chars escaped with htmlenc: ampersand in connection string",
			template:    `<add key="ConnectionString" value="{{ secret "conn_str" | htmlenc }}" />`,
			secretsMap: map[string]*Secret{
				"conn_str": {Alias: "conn_str", Value: "Server=db;Password=P@ss&word!"},
			},
			// htmlenc escapes & → &amp;, producing valid XML
			wantOutput: `<add key="ConnectionString" value="Server=db;Password=P@ss&amp;word!" />`,
		},
		{
			description: "special chars escaped with htmlenc: angle brackets and quotes",
			template:    `<add key="Filter" value="{{ secret "filter" | htmlenc }}" />`,
			secretsMap: map[string]*Secret{
				"filter": {Alias: "filter", Value: `<script>alert("xss")</script>`},
			},
			wantOutput: `<add key="Filter" value="&lt;script&gt;alert(&#34;xss&#34;)&lt;/script&gt;" />`,
		},
		{
			description: "Maven settings.xml: password in text node escaped with htmlenc",
			template: `<server>
  <id>{{ secret "server_id" }}</id>
  <username>{{ secret "username" }}</username>
  <password>{{ secret "password" | htmlenc }}</password>
</server>`,
			secretsMap: map[string]*Secret{
				"server_id": {Alias: "server_id", Value: "central"},
				"username":  {Alias: "username", Value: "deploy-bot"},
				// password contains & to verify text-node escaping, not just attribute escaping
				"password": {Alias: "password", Value: "P@ss&phrase!"},
			},
			wantOutput: `<server>
  <id>central</id>
  <username>deploy-bot</username>
  <password>P@ss&amp;phrase!</password>
</server>`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			tpl := GetTemplate("test", tt.secretsMap)
			parsedTpl, err := tpl.Parse(tt.template)
			require.NoError(t, err)

			tplData := TemplateData{SecretsMap: tt.secretsMap}
			buf, err := RenderFile(parsedTpl, tplData)
			require.NoError(t, err)

			output := buf.String()
			assert.Equal(t, tt.wantOutput, output)
			assertValidXML(t, output)
		})
	}
}
