package filetemplates

import (
	"testing"
	"text/template"

	"github.com/stretchr/testify/assert"
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
