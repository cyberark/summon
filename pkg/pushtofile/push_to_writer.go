package pushtofile

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/cyberark/summon/pkg/atomicwriter"
	filetemplates "github.com/cyberark/summon/pkg/file_templates"
)

// pushToWriterFunc is the func definition for pushToWriter. It allows switching out pushToWriter
// for a mock implementation
type pushToWriterFunc func(
	writer io.Writer,
	filePath string,
	fileTemplate string,
	fileSecrets []*filetemplates.Secret,
) error

// openWriteCloserFunc is the func definition for openFileAsWriteCloser. It allows switching
// out openFileAsWriteCloser for a mock implementation
type openWriteCloserFunc func(
	path string,
	permissions os.FileMode,
	overwrite bool,
) (io.WriteCloser, error)

// openFileAsWriteCloser opens a file to write-to with some permissions.
// When overwrite is false and the file already exists, it returns an error.
func openFileAsWriteCloser(path string, permissions os.FileMode, overwrite bool) (io.WriteCloser, error) {
	dir := filepath.Dir(path)

	// Create the file here to capture any errors, instead of in atomicWriter.Close() which may be deferred and ignored
	err := os.MkdirAll(dir, os.ModePerm)
	if err != nil {
		return nil, fmt.Errorf("unable to mkdir when opening file to write at %q: %s", path, err)
	}

	// Check if file already exists when overwrite is not enabled
	if !overwrite {
		if _, err := os.Stat(path); err == nil {
			return nil, fmt.Errorf("file already exists at %q and overwrite is not enabled", path)
		}
	}

	// Return an instance of an atomic writer
	atomicWriter := atomicwriter.NewAtomicWriter(path, permissions)

	return atomicWriter, nil
}

// pushToWriter takes a (file's) path, template and secrets, and processes the template
// to generate text content that is pushed to a writer. push-to-file wraps around this.
func pushToWriter(
	writer io.Writer,
	filePath string,
	fileTemplate string,
	fileSecrets []*filetemplates.Secret,
) error {
	secretsMap := map[string]*filetemplates.Secret{}
	for _, s := range fileSecrets {
		secretsMap[s.Alias] = s
	}

	tpl, err := filetemplates.GetTemplate(filePath, secretsMap).Parse(fileTemplate)
	if err != nil {
		return err
	}

	// Render the secret file content
	tplData := filetemplates.TemplateData{
		SecretsArray: fileSecrets,
		SecretsMap:   secretsMap,
	}
	fileContent, err := filetemplates.RenderFile(tpl, tplData)
	if err != nil {
		return err
	}

	return writeContent(writer, fileContent)
}

func writeContent(writer io.Writer, fileContent *bytes.Buffer) error {
	_, err := writer.Write(fileContent.Bytes())
	return err
}
