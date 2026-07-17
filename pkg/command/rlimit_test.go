package command

import (
	"bytes"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSuppressCoreDumps(t *testing.T) {
	cases := []struct {
		name       string
		limitErr   error
		wantLog    bool
		wantSubstr []string
	}{
		{
			name:     "success is silent",
			limitErr: nil,
			wantLog:  false,
		},
		{
			name:     "failure logs at debug and continues",
			limitErr: errors.New("operation not permitted"),
			wantLog:  true,
			wantSubstr: []string{
				"Failed to suppress core dumps via Setrlimit; continuing",
				"operation not permitted",
				"level=DEBUG",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			original := applyCoreDumpLimit
			defer func() { applyCoreDumpLimit = original }()

			originalHandler := slog.Default().Handler()
			defer slog.SetDefault(slog.New(originalHandler))

			var buf bytes.Buffer
			slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))

			applyCoreDumpLimit = func() error {
				return tc.limitErr
			}

			assert.NotPanics(t, suppressCoreDumps)

			if !tc.wantLog {
				assert.Empty(t, buf.String())
				return
			}

			output := buf.String()
			for _, substr := range tc.wantSubstr {
				assert.Contains(t, output, substr)
			}
		})
	}
}
