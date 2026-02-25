package pushtofile

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	filetemplates "github.com/cyberark/summon/pkg/file_templates"
	"github.com/cyberark/summon/pkg/provider"
	"github.com/cyberark/summon/pkg/secretsyml"
)

const defaultFilePermissions os.FileMode = 0o600
const maxFilenameLen = 255

type SecretFile struct {
	FileConfig secretsyml.FileConfig
	Ignores    []string
	IgnoreAll  bool
}

func (secretFile *SecretFile) SecretSpecs() secretsyml.SecretsMap {
	return secretFile.FileConfig.Secrets.(secretsyml.SecretsMap)
}

func (secretFile *SecretFile) Write(providerResults []provider.Result) (absolutePath string, err error) {
	return secretFile.writeWithDeps(openFileAsWriteCloser, pushToWriter, providerResults)
}

func (secretFile *SecretFile) writeWithDeps(
	depOpenWriteCloser openWriteCloserFunc,
	depPushToWriter pushToWriterFunc,
	providerResults []provider.Result,
) (absolutePath string, err error) {
	err = secretFile.validate()
	if err != nil {
		return "", err
	}

	// Make sure all the secret specs are accounted for
	secrets, err := validateSecretsAgainstSpecs(providerResults, secretFile.SecretSpecs(), secretFile.Ignores, secretFile.IgnoreAll)
	if err != nil {
		return "", err
	}

	// Determine file template from
	// 1. File template
	// 2. File format
	// 3. Secret specs (user to validate file template)
	fileTemplate, err := maybeFileTemplateFromFormat(
		secretFile.FileConfig.Template,
		secretFile.FileConfig.Format,
		secretFile.SecretSpecs(),
	)
	if err != nil {
		return "", err
	}

	// Open and push to file
	absolutePath, err = secretFile.absoluteFilePath()
	if err != nil {
		return "", err
	}
	filePermissions := secretFile.FileConfig.Permissions
	if filePermissions == 0 {
		filePermissions = defaultFilePermissions
	}
	wc, err := depOpenWriteCloser(absolutePath, filePermissions, secretFile.FileConfig.Overwrite)
	if err != nil {
		return "", err
	}
	defer func() {
		_ = wc.Close()
	}()

	maskError := fmt.Errorf("failed to execute template, with secret values, on push to file %q", secretFile.FileConfig.Path)
	defer func() {
		if r := recover(); r != nil {
			err = maskError
		}
	}()
	err = depPushToWriter(
		wc,
		secretFile.FileConfig.Path,
		fileTemplate,
		secrets,
	)
	if err != nil {
		err = maskError
	}
	return absolutePath, err
}

func (secretFile *SecretFile) absoluteFilePath() (string, error) {
	filePath := secretFile.FileConfig.Path

	pathContainsFilename := !strings.HasSuffix(filePath, "/") && len(filePath) > 0

	if !pathContainsFilename {
		return "", fmt.Errorf(
			"provided filepath %q must contain a filename",
			filePath,
		)
	}

	// Clean the path to resolve any ".." or "." segments
	filePath = filepath.Clean(filePath)

	// If the file path is not absolute, make it absolute by joining it with the current working directory
	if !filepath.IsAbs(filePath) {
		pwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("relative file path %q provided but current working directory cannot be determined: %s", filePath, err)
		}
		filePath = filepath.Join(pwd, filePath)
	}

	// Filename cannot be longer than allowed by the filesystem
	_, filename := filepath.Split(filePath)
	if len(filename) > maxFilenameLen {
		return "", fmt.Errorf(
			"filename %q for provided filepath must not be longer than %d characters",
			filename,
			maxFilenameLen,
		)
	}

	return filePath, nil
}

func (secretFile *SecretFile) validate() error {
	fileFormat := secretFile.FileConfig.Format
	fileTemplate := secretFile.FileConfig.Template
	filePath := secretFile.FileConfig.Path
	secretSpecs := secretFile.SecretSpecs()

	if len(fileFormat) > 0 && fileFormat != "template" {
		_, err := FileTemplateForFormat(fileFormat, secretSpecs)
		if err != nil {
			return fmt.Errorf(
				"unable to process file %q into file format %q: %s",
				filePath,
				fileFormat,
				err,
			)
		}
	}

	// First-pass at provided template rendering with dummy secret values
	// This first-pass is limited for templates that branch conditionally on secret values
	// Relying logically on specific secret values should be avoided
	if len(fileTemplate) > 0 {
		dummySecrets := []*filetemplates.Secret{}
		for alias := range secretSpecs {
			dummySecrets = append(dummySecrets, &filetemplates.Secret{Alias: alias, Value: "REDACTED"})
		}

		err := pushToWriter(io.Discard, filePath, fileTemplate, dummySecrets)
		if err != nil {
			return fmt.Errorf(
				`unable to use file template for file %q: %s`,
				filePath,
				err,
			)
		}
	}

	return nil
}

func validateSecretsAgainstSpecs(
	providerResults []provider.Result,
	specs secretsyml.SecretsMap,
	ignores []string,
	ignoreAll bool,
) ([]*filetemplates.Secret, error) {
	secrets := make([]*filetemplates.Secret, 0, len(providerResults))
	var errorResults []provider.Result
	ignoredAliases := make(map[string]struct{})

	for _, result := range providerResults {
		// result.Key is the alias (e.g., "DB_USERNAME")
		alias := result.Key

		// Handle provider errors with ignore logic
		if result.Error != nil {
			if shouldIgnoreAlias(alias, ignores, ignoreAll) {
				ignoredAliases[alias] = struct{}{}
				continue
			}
			errorResults = append(errorResults, result)
			continue
		}

		// Check if this alias exists in specs
		if _, hasAlias := specs[alias]; !hasAlias {
			slog.Warn("alias not found in specs, skipping", "alias", alias)
			continue
		}

		secrets = append(secrets, &filetemplates.Secret{
			Alias: alias,
			Value: result.Value,
		})
	}

	// If there are any non-ignored errors, return the first one
	if len(errorResults) > 0 {
		firstError := errorResults[0]
		slog.Debug("Error fetching secret", "name", firstError.Key, "error", firstError.Error)
		return nil, fmt.Errorf("Error fetching secret: %w", firstError.Error)
	}

	// Check for missing aliases (specs that don't have corresponding results)
	secretKeys := make(map[string]struct{})
	for _, secret := range secrets {
		secretKeys[secret.Alias] = struct{}{}
	}

	var missingAliases []string
	for alias := range specs {
		_, hasSecret := secretKeys[alias]
		_, wasIgnored := ignoredAliases[alias]
		if !hasSecret && !wasIgnored {
			missingAliases = append(missingAliases, alias)
		}
	}

	sort.Strings(missingAliases)

	if len(missingAliases) > 0 {
		return nil, fmt.Errorf("some secret specs are not present in secrets: %q", strings.Join(missingAliases, ", "))
	}

	// Sort the secrets by alias for deterministic output
	sort.Slice(secrets, func(i, j int) bool {
		return secrets[i].Alias < secrets[j].Alias
	})

	return secrets, nil
}

func shouldIgnoreAlias(alias string, ignores []string, ignoreAll bool) bool {
	if ignoreAll {
		return true
	}
	return slices.Contains(ignores, alias)
}

func maybeFileTemplateFromFormat(
	fileTemplate string,
	fileFormat string,
	secretSpecs secretsyml.SecretsMap,
) (string, error) {
	// TODO: Detect format from filename

	// Default to "yaml" file format
	if fileTemplate == "" && fileFormat == "" {
		fileFormat = "yaml"
	}

	// fileFormat is used to set fileTemplate when fileTemplate is not already set
	if len(fileTemplate) == 0 {
		var err error

		fileTemplate, err = FileTemplateForFormat(
			fileFormat,
			secretSpecs,
		)
		if err != nil {
			return "", err
		}
	}

	return fileTemplate, nil
}
