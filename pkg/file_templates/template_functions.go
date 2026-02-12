package filetemplates

import "encoding/base64"

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
