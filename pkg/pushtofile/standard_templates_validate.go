package pushtofile

import (
	"fmt"
	"regexp"
)

const (
	maxYAMLKeyLen = 1024
	maxJSONKeyLen = 2097152
)

var (
	propertyVarNameRegex = regexp.MustCompile("^[a-zA-Z_][a-zA-Z0-9_.]*$")
	bashVarNameRegex     = regexp.MustCompile("^[a-zA-Z_][a-zA-Z0-9_]*$")
)

func validateYAMLKey(key string) error {
	if len(key) > maxYAMLKeyLen {
		return fmt.Errorf("the key '%s' is too long for YAML", key)
	}
	for _, c := range key {
		if !isValidYAMLChar(c) {
			return fmt.Errorf("invalid YAML character: '%c'", c)
		}
	}
	return nil
}

func isValidYAMLChar(c rune) bool {
	// Checks whether a character is in the YAML valid character set as
	// defined here: https://yaml.org/spec/1.2.2/#51-character-set
	switch {
	case c == '\u0009':
		return true // tab
	case c == '\u000A':
		return true // LF
	case c == '\u000D':
		return true // CR
	case c >= '\u0020' && c <= '\u007E':
		return true // Printable ASCII
	case c == '\u0085':
		return true // Next Line (NEL)
	case c >= '\u00A0' && c <= '\uD7FF':
		return true // Basic Multilingual Plane (BMP)
	case c >= '\uE000' && c <= '\uFFFD':
		return true // Additional Unicode Areas
	case c >= '\U00010000' && c <= '\U0010FFFF':
		return true // 32 bit
	default:
		return false
	}
}

func validateJSONKey(key string) error {
	if len(key) > maxJSONKeyLen {
		return fmt.Errorf("the key '%s' is too long for JSON", key)
	}
	for _, c := range key {
		if !isValidJSONChar(c) {
			return fmt.Errorf("invalid JSON character: '%c'", c)
		}
	}
	return nil
}

func isValidJSONChar(c rune) bool {
	// Checks whether a character is in the JSON valid character set as
	// defined here: https://www.json.org/json-en.html
	// This document specifies that any characters are valid except:
	//   - Control characters (0x00-0x1F and 0x7f [DEL])
	//   - Double quote (")
	//   - Backslash (\)
	switch {
	case c >= '\u0000' && c <= '\u001F':
		return false // Control characters other than DEL
	case c == '\u007F':
		return false // DEL
	case c == rune('"'):
		return false // Double quote
	case c == rune('\\'):
		return false // Backslash
	default:
		return true
	}
}

func validatePropertyVarName(name string) error {
	if !propertyVarNameRegex.MatchString(name) {
		explanation := "property names can only include alphanumerics, dots and underscores, with first char being a non-digit/non-dot"
		return fmt.Errorf("invalid alias %q: %s", name, explanation)
	}
	return nil
}

func validateBashVarName(name string) error {
	if !bashVarNameRegex.MatchString(name) {
		explanation := "variable names can only include alphanumerics and underscores, with first char being a non-digit"
		return fmt.Errorf("invalid alias %q: %s", name, explanation)
	}
	return nil
}
