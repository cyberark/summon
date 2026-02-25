package pushtofile

import (
	"bytes"
	"io"
	"os"

	filetemplates "github.com/cyberark/summon/pkg/file_templates"
)

type ClosableBuffer struct {
	bytes.Buffer
	CloseErr error
}

func (c ClosableBuffer) Close() error { return c.CloseErr }

// pushToWriterFunc
type pushToWriterArgs struct {
	writer       io.Writer
	filePath     string
	fileTemplate string
	fileSecrets  []*filetemplates.Secret
}

type pushToWriterSpy struct {
	args   pushToWriterArgs
	err    error
	_calls int
}

func (spy *pushToWriterSpy) Call(
	writer io.Writer,
	filePath string,
	fileTemplate string,
	fileSecrets []*filetemplates.Secret,
) error {
	spy._calls++
	// This is to ensure the spy is only ever used once!
	if spy._calls > 1 {
		panic("spy called more than once")
	}

	spy.args = pushToWriterArgs{
		writer:       writer,
		filePath:     filePath,
		fileTemplate: fileTemplate,
		fileSecrets:  fileSecrets,
	}

	return spy.err
}

// openWriteCloserFunc
type openWriteCloserArgs struct {
	path        string
	permissions os.FileMode
	overwrite   bool
}

type openWriteCloserSpy struct {
	args        openWriteCloserArgs
	writeCloser io.WriteCloser
	err         error
	_calls      int
}

func (spy *openWriteCloserSpy) Call(path string, permissions os.FileMode, overwrite bool) (io.WriteCloser, error) {
	spy._calls++
	// This is to ensure the spy is only ever used once!
	if spy._calls > 1 {
		panic("spy called more than once")
	}

	spy.args = openWriteCloserArgs{
		path:        path,
		permissions: permissions,
		overwrite:   overwrite,
	}

	return spy.writeCloser, spy.err
}
