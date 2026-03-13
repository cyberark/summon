package pushtofile

const jsonTemplate = `{
{{- range $index, $secret := .SecretsArray }}
{{- if $index }},{{ end }}
{{- printf "%q" $secret.Alias }}:{{ printf "%q" $secret.Value }}
{{- end -}}
}`
const yamlTemplate = `
{{- range $index, $secret := .SecretsArray }}
{{- if $index }}{{ "\n" }}{{ end }}
{{- printf "%q" $secret.Alias }}: {{ printf "%q" $secret.Value -}}
{{- end -}}
`
const dotenvTemplate = `
{{- range $index, $secret := .SecretsArray -}}
{{- if $index }}{{ "\n" }}{{ end }}
{{- $secret.Alias }}={{ printf "%q" $secret.Value }}
{{- end -}}
`
const bashTemplate = `
{{- range $index, $secret := .SecretsArray -}}
{{- if $index }}{{ "\n" }}{{ end }}
{{- ""}}export {{ $secret.Alias }}={{ printf "%q" $secret.Value }}
{{- end -}}
`
