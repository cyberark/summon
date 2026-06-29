package filetemplates

import "testing"

func TestPropertiesencTemplateFunc(t *testing.T) {
	testCases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "unicode passes through unchanged",
			input: "café",
			want:  "café",
		},
		{
			name:  "non-latin unicode passes through unchanged",
			input: "密码",
			want:  "密码",
		},
		{
			name:  "trailing spaces are preserved",
			input: "value   ",
			want:  "value   ",
		},
		{
			name:  "inner spaces are not escaped",
			input: "hello world",
			want:  "hello world",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := propertiesencTemplateFunc(tc.input)
			if got != tc.want {
				t.Fatalf("propertiesenc(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}
