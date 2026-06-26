package filetemplates

import (
	"encoding/base64"
	"html"
	"strings"
)

// Define template functions that don't need access to secrets in this file
// to keep the push_to_writer.go file cleaner with only the functions that
// require access to secrets.

// b64enc is a custom template function for performing a base64 encode
// on a secret value.
func b64encTemplateFunc(value string) string {
	return base64.StdEncoding.EncodeToString([]byte(value))
}

// b64dec is a custom template function for performing a base64 decode
// on a secret value.
func b64decTemplateFunc(encValue string) string {
	decValue, err := base64.StdEncoding.DecodeString(encValue)
	if err == nil {
		return string(decValue)
	}

	// Panic in a template function is captured as an error
	// when the template is executed.
	panic("value could not be base64 decoded")
}

// htmlenc is a custom template function that escapes a string for safe
// embedding in HTML or XML attribute values and text content.
// It replaces &, <, >, ", and ' with their character entity equivalents,
// producing well-formed XML output.
func htmlencTemplateFunc(value string) string {
	return html.EscapeString(value)
}

// propertiesenc escapes a string for use as a Java .properties value.
//
// The properties format does not use quotes as value delimiters, so literal
// double quotes are preserved. Characters that Java properties treats as
// syntactically meaningful or escape sequences are backslash-escaped.
func propertiesencTemplateFunc(value string) string {
	var b strings.Builder
	b.Grow(len(value))

	for i, r := range value {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		case '\f':
			b.WriteString(`\f`)
		case ' ':
			if i == 0 {
				b.WriteByte('\\')
			}
			b.WriteRune(r)
		case '=', ':', '#', '!':
			b.WriteByte('\\')
			b.WriteRune(r)
		default:
			b.WriteRune(r)
		}
	}

	return b.String()
}
