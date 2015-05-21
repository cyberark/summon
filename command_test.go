package cauldron

import (
	"bytes"
	"testing"
)

// E2E test for the command line interface
func TestStart(t *testing.T) {
	yamlContent := "'AWS_PEM: !file $policy/aws/iam/user/robot/access_key_id'"
	args = []string{"cauldron-testing", "run", "--yaml", yamlContent, "printenv", "AWS_PEM"}
	buf := &bytes.Buffer{}
	writer = buf

	cli := CLI{"/dev/null"}
	err := cli.Start()
	if err != nil {
		t.Error(err.Error())
	}

	//TODO: buf doesn't have content, find out why
}
