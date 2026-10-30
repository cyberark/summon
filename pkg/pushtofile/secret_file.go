package pushtofile

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/cyberark/summon/internal/memzero"
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
	// Daemon marks a long-running (--refresh) caller that rewrites the file
	// repeatedly: the destination directory must be owned by the current user
	// and not group/world-writable, a symlinked destination is refused, and
	// the directory is fsynced after each rename.
	Daemon bool
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
	plan, err := secretFile.plan(providerResults)
	if err != nil {
		return "", err
	}
	return plan.absolutePath, secretFile.pushPlan(
		plan, depOpenWriteCloser, depPushToWriter, secretFile.FileConfig.Overwrite)
}

// WriteIfChanged renders the file and rewrites it only when the SHA-256 of the
// rendered content differs from prevChecksum. An empty prevChecksum means no
// earlier write: the file is written and the configured overwrite setting is
// respected. Once a checksum exists Summon owns the file, so later rewrites
// always overwrite. It returns the absolute path, the checksum of the rendered
// content, and whether the file was written.
func (secretFile *SecretFile) WriteIfChanged(
	providerResults []provider.Result,
	prevChecksum string,
) (absolutePath string, checksum string, written bool, err error) {
	open := openFileAsWriteCloser
	if secretFile.Daemon {
		open = openFileAsDaemonWriteCloser
	}
	return secretFile.writeIfChanged(providerResults, prevChecksum, open)
}

func (secretFile *SecretFile) writeIfChanged(
	providerResults []provider.Result,
	prevChecksum string,
	depOpenWriteCloser openWriteCloserFunc,
) (absolutePath string, checksum string, written bool, err error) {
	plan, err := secretFile.plan(providerResults)
	if err != nil {
		return "", "", false, err
	}

	var rendered bytes.Buffer
	defer func() { memzero.Clear(rendered.Bytes()) }()
	if err = secretFile.renderPlan(plan, pushToWriter, &rendered); err != nil {
		return "", "", false, err
	}
	sum := sha256.Sum256(rendered.Bytes())
	checksum = hex.EncodeToString(sum[:])
	if checksum == prevChecksum {
		return plan.absolutePath, checksum, false, nil
	}

	overwrite := secretFile.FileConfig.Overwrite || prevChecksum != ""
	wc, err := depOpenWriteCloser(plan.absolutePath, plan.permissions, overwrite)
	if err != nil {
		return "", "", false, err
	}
	if _, err = wc.Write(rendered.Bytes()); err != nil {
		_ = wc.Close()
		return "", "", false, fmt.Errorf("failed to write push to file %q: %w", secretFile.FileConfig.Path, err)
	}
	// The atomic writer only renames the temp file into place on Close, so a
	// Close error means the destination was not updated.
	if err = wc.Close(); err != nil {
		return "", "", false, fmt.Errorf("failed to finalize push to file %q: %w", secretFile.FileConfig.Path, err)
	}
	return plan.absolutePath, checksum, true, nil
}

// writePlan holds everything resolved before any content is rendered.
type writePlan struct {
	secrets      []*filetemplates.Secret
	fileTemplate string
	absolutePath string
	permissions  os.FileMode
}

// plan validates the file config against the provider results and resolves
// the template, destination path and permissions.
func (secretFile *SecretFile) plan(providerResults []provider.Result) (*writePlan, error) {
	err := secretFile.validate()
	if err != nil {
		return nil, err
	}

	// Make sure all the secret specs are accounted for
	secrets, err := validateSecretsAgainstSpecs(providerResults, secretFile.SecretSpecs(), secretFile.Ignores, secretFile.IgnoreAll)
	if err != nil {
		return nil, err
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
		return nil, err
	}

	absolutePath, err := secretFile.absoluteFilePath()
	if err != nil {
		return nil, err
	}
	filePermissions := secretFile.FileConfig.Permissions
	if filePermissions == 0 {
		filePermissions = defaultFilePermissions
	}
	return &writePlan{
		secrets:      secrets,
		fileTemplate: fileTemplate,
		absolutePath: absolutePath,
		permissions:  filePermissions,
	}, nil
}

// pushPlan opens the destination and pushes the rendered plan to it.
func (secretFile *SecretFile) pushPlan(
	plan *writePlan,
	depOpenWriteCloser openWriteCloserFunc,
	depPushToWriter pushToWriterFunc,
	overwrite bool,
) error {
	wc, err := depOpenWriteCloser(plan.absolutePath, plan.permissions, overwrite)
	if err != nil {
		return err
	}
	err = secretFile.renderPlan(plan, depPushToWriter, wc)
	// Close performs the atomic rename; its failure means nothing was written.
	if closeErr := wc.Close(); err == nil && closeErr != nil {
		err = fmt.Errorf("failed to finalize push to file %q: %w", secretFile.FileConfig.Path, closeErr)
	}
	return err
}

// renderPlan renders the plan's template into w. Errors and panics are masked
// because template execution has access to the secret values.
func (secretFile *SecretFile) renderPlan(
	plan *writePlan,
	depPushToWriter pushToWriterFunc,
	w io.Writer,
) (err error) {
	maskError := fmt.Errorf("failed to execute template, with secret values, on push to file %q", secretFile.FileConfig.Path)
	defer func() {
		if r := recover(); r != nil {
			err = maskError
		}
	}()
	err = depPushToWriter(
		w,
		secretFile.FileConfig.Path,
		plan.fileTemplate,
		plan.secrets,
	)
	if err != nil {
		err = maskError
	}
	return err
}

func (secretFile *SecretFile) absoluteFilePath() (string, error) {
	filePath := secretFile.FileConfig.Path

	// os.IsPathSeparator also matches a trailing backslash on Windows.
	pathContainsFilename := len(filePath) > 0 && !os.IsPathSeparator(filePath[len(filePath)-1])

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

	// Require template content when format is "template"
	if fileFormat == "template" && len(fileTemplate) == 0 {
		return fmt.Errorf(
			"file config for path %q specifies format \"template\" but no template content was provided",
			filePath,
		)
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

		// Check if this alias exists in specs — if not, it may belong
		// to another file's spec, so skip silently.
		if _, hasAlias := specs[alias]; !hasAlias {
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

	// fileFormat is used to set fileTemplate when fileTemplate is not already set.
	// The default format ("yaml") is applied upstream in processFileConfigWithNode.
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
