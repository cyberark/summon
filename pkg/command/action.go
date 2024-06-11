package command

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	prov "github.com/cyberark/summon/pkg/provider"
	"github.com/cyberark/summon/pkg/summon"
	"github.com/urfave/cli"
)

// Action is the runner for the main program logic
var Action = func(c *cli.Context) {
	if !c.Args().Present() && !c.Bool("all-provider-versions") {
		fmt.Println("Enter a subprocess to run!")
		os.Exit(127)
	}

	provider, err := prov.Resolve(c.String("provider"))
	// It's okay to not throw this error here, because `Resolve()` throws an
	// error if there are multiple unspecified providers. `all-provider-versions`
	// doesn't care about this and just looks in the default provider dir
	if err != nil && !c.Bool("all-provider-versions") {
		fmt.Println(err.Error())
		os.Exit(127)
	}

	if c.Bool("all-provider-versions") {
		if err := runPrintProviderVersions(); err != nil {
			fmt.Println(err.Error())
			os.Exit(127)
		}
		return
	}

	code, err := summon.RunSubprocess(&summon.SubprocessConfig{
		Args:        c.Args(),
		Environment: c.String("environment"),
		Filepath:    c.String("f"),
		YamlInline:  c.String("yaml"),
		Ignores:     c.StringSlice("ignore"),
		IgnoreAll:   c.Bool("ignore-all"),
		RecurseUp:   c.Bool("up"),
		Subs:        c.StringSlice("D"),
		Provider:    provider,
		FetchSecret: func(secretId string) ([]byte, error) {
			s, err := prov.Call(provider, secretId)
			return []byte(s), err
		},
	})

	if err != nil {
		fmt.Println(err.Error())
		os.Exit(127)
	}

	os.Exit(code)
}

func runPrintProviderVersions() error {
	defaultPath, err := prov.GetDefaultPath()
	if err != nil {
		return err
	}
	output, err := printProviderVersions(defaultPath)
	if err != nil {
		return err
	}

	fmt.Print(output)
	return nil
}

// printProviderVersions returns a string of all provider versions
func printProviderVersions(providerPath string) (string, error) {
	var providerVersions bytes.Buffer

	providerVersions.WriteString(fmt.Sprintf("Provider versions in %s:\n", providerPath))

	providers, err := prov.GetAllProviders(providerPath)
	if err != nil {
		return "", err
	}

	for _, provider := range providers {
		version, err := exec.Command(filepath.Join(providerPath, provider), "--version").Output()
		if err != nil {
			providerVersions.WriteString(fmt.Sprintf("%s: unknown version\n", provider))
			continue
		}

		versionString := fmt.Sprintf("%s", version)
		versionString = strings.TrimSpace(versionString)

		providerVersions.WriteString(fmt.Sprintf("%s version %s\n", provider, versionString))
	}

	return providerVersions.String(), nil
}
