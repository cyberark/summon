package filetemplates

import (
	"bytes"
	"fmt"
	"text/template"
)

// Secret is a resolved key/value pair ready for template rendering.
// It is deliberately minimal — no error or tag metadata — because by
// this point all fetching, error handling, and ignore logic are done
// (see provider.Result for the previous stage). Templates only need
// the alias to look up a value.
type Secret struct {
	Alias string // The name used to reference this secret in templates.
	Value string // The resolved secret content.
}

// templateData describes the form in which data is presented to file templates
type TemplateData struct {
	SecretsArray []*Secret
	SecretsMap   map[string]*Secret
}

func RenderFile(tpl *template.Template, tplData TemplateData) (*bytes.Buffer, error) {
	buf := &bytes.Buffer{}
	err := tpl.Execute(buf, tplData)
	return buf, err
}

func GetTemplate(name string, secretsMap map[string]*Secret) *template.Template {
	return template.New(name).Funcs(template.FuncMap{
		// secret is a custom utility function for streamlined access to secret values.
		// It panics for secrets aliases not specified on the file.
		"secret": func(alias string) string {
			v, ok := secretsMap[alias]
			if ok {
				return v.Value
			}

			// Panic in a template function is captured as an error
			// when the template is executed.
			panic(fmt.Sprintf("secret alias %q not present in specified secrets for file", alias))
		},
		"b64enc":  b64encTemplateFunc,
		"b64dec":  b64decTemplateFunc,
		"htmlenc": htmlencTemplateFunc,
	})
}
