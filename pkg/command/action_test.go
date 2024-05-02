package command

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	_ "golang.org/x/net/context"
)

func TestPrintProviderVersions(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping long-running test.")
	}

	t.Run("printProviderVersions should return a string of all of the providers in the defaultPath", func(t *testing.T) {
		pathTo, err := os.Getwd()
		assert.NoError(t, err)
		pathToTest := filepath.Join(pathTo, "testversions")

		//test1 - regular formating and appending of version # to string
		//test2 - chopping off of trailing newline
		//test3 - failed `--version` call
		output, err := printProviderVersions(pathToTest)
		assert.NoError(t, err)

		expected := `Provider versions in /summon/pkg/command/testversions:
testprovider version 1.2.3
testprovider-noversionsupport: unknown version
testprovider-trailingnewline version 3.2.1
`

		assert.Equal(t, expected, output)
	})
}
