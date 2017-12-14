package command

import (
	"fmt"
	"github.com/codegangsta/cli"
	prov "github.com/cyberark/summon/provider"
	"github.com/cyberark/summon/secretsyml"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"reflect"
	"time"
	"sort"
	"hash/fnv"
	"os/signal"
	"text/template"
	"bytes"
)

type ActionConfig struct {
	Args              []string
	Provider          string
	Filepath          string
	TemplatePath      string
	YamlInline        string
	Subs              map[string]string
	Ignores           []string
	Environment       string
	WatchPollInterval time.Duration
	WatchMode         bool
}

const ENV_FILE_MAGIC = "@SUMMONENVFILE"

var Action = func(c *cli.Context) {
	if !c.Args().Present() {
		fmt.Println("Enter a subprocess to run!")
		os.Exit(127)
	}

	provider, err := prov.Resolve(c.String("provider"))
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(127)
	}

	runAction(&ActionConfig{
		Args:        c.Args(),
		Provider:    provider,
		Environment: c.String("environment"),
		Filepath:    c.String("f"),
		TemplatePath:    c.String("t"),
		YamlInline:  c.String("yaml"),
		WatchMode:  c.Bool("w"),
		WatchPollInterval:  time.Duration(c.Int("watch-poll-interval")) * time.Millisecond,
		Ignores:     c.StringSlice("ignore"),
		Subs:        convertSubsToMap(c.StringSlice("D")),
	})
}

type ProviderResult struct {
	Spec secretsyml.SecretSpec
	Key string
	Value string
	Error error
}

func runProvider(secrets secretsyml.SecretsMap, provider string) []ProviderResult {
	var err error

	// Run provider calls concurrently
	results := make(chan ProviderResult, len(secrets))
	var wg sync.WaitGroup

	for key, spec := range secrets {
		wg.Add(1)
		go func(key string, spec secretsyml.SecretSpec) {
			var value string
			if spec.IsVar() {
				value, err = prov.Call(provider, spec.Path)
				if err != nil {
					results <- ProviderResult{Key: key, Error: err}
					wg.Done()
					return
				}
			} else {
				// If the spec isn't a variable, use its value as-is
				value = spec.Path
			}

			results <- ProviderResult{Key: key, Value: value, Spec: spec}
			wg.Done()
		}(key, spec)
	}
	wg.Wait()
	close(results)

	resultsSlice := make([]ProviderResult, 0)

	for result := range results {
		resultsSlice = append(resultsSlice, result)
	}
	return resultsSlice
}

func getProviderResults(ac *ActionConfig) ([]ProviderResult, string, error) {
	var (
		secrets secretsyml.SecretsMap
		err     error
	)

	switch ac.YamlInline {
	case "":
		secrets, err = secretsyml.ParseFromFile(ac.Filepath, ac.Environment, ac.Subs)
	default:
		secrets, err = secretsyml.ParseFromString(ac.YamlInline, ac.Environment, ac.Subs)
	}

	if err != nil {
		return nil, "", err
	}

	results := runProvider(secrets, ac.Provider)

EnvLoop:
	for _, envvar := range results {
		if envvar.Error != nil {
			for i := range ac.Ignores {
				if ac.Ignores[i] == envvar.Key {
					continue EnvLoop
				}
			}
			return nil, "Error fetching variable " + envvar.Key, envvar.Error
		}
	}

	return results, "", nil
}

func singleQuoteEscape(line string) string {
	line = strings.Replace(line, string('\''), `'\''`, -1)

	return line
}

func ProviderResultsHash(prs []ProviderResult) uint32 {
	s := make([]string, 0)
	for _, pr := range prs {
		s = append(s, fmt.Sprintf("%s=%s", pr.Key, pr.Value))
	}

	sort.Strings(s)
	h := fnv.New32a()
	h.Write([]byte(strings.Join(s, ",")))
	return h.Sum32()

	// TODO: same hash but hey it's not meant for secrecy?
	//fmt.Println(hash([]string{"a", "b", "c,x"}))
	//fmt.Println(hash([]string{"b", "a", "c", "x"}))
}

// runAction encapsulates the logic of Action without cli Context for easier testing
func runAction(ac *ActionConfig) {

	var (
		previousSecretsHash uint32
		currentTempFactory *TempFactory
		currentRunner *exec.Cmd
	)
	done := make(chan bool)

	// clean up tempfiles when summon is abruptly cut
	// https://stackoverflow.com/questions/41432193/how-to-delete-a-file-using-golang-on-program-exit
	// maybe also clean up tmp files from summon when summon is run

	gracefulStop := make(chan os.Signal)
	signal.Notify(gracefulStop, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-gracefulStop
		currentTempFactory.Cleanup()
		os.Exit(0)
	}()

	cleanUp := func() {}
	for {
		results, out, err := getProviderResults(ac)
		if err != nil {
			code, err := returnStatusOfError(err)
			currentRunner.Process.Kill()

			if err != nil {
				fmt.Println(out + ": " + err.Error())
				os.Exit(127)
			}

			os.Exit(code)
			return
		}

		currentSecretsHash := ProviderResultsHash(results)

		secretsMapHasBeenUpdated := !reflect.DeepEqual(currentSecretsHash, previousSecretsHash)

		if secretsMapHasBeenUpdated {
			// restart process
			cleanUp()

			// create temporary files
			var env []string
			tempFactory := NewTempFactory("")
			// for clean up
			currentTempFactory = &tempFactory

			for _, result := range results {
				env = append(env, formatForEnv(result.Key, result.Value, result.Spec, &tempFactory))
			}

			setupEnvFile(ac.Args, ac.TemplatePath, results,  &tempFactory)

			runner := runSubCommand(ac.Args, append(os.Environ(), env...))
			err = runner.Start()
			currentRunner = runner
			respondToError("application starting error: ", err)

			go func() {
				defer tempFactory.Cleanup()
				runner.Wait()
				if !ac.WatchMode {
					done <- true
				}
				// TODO: currently if a long running application dies abruptly nothing happens
				// currently does no error handling.
				// this makes sense because error handling for your application shouldn't be something that summon does for you
				// we could try a couple times to restart then exit if it is a not so innocent error
				// This only applies to long lived processes. Something like env will exit immediately
				//err := runner.Wait()
				//respondToError("application running error: ", err)
			}()

			cleanUp = func() {
				runner.Process.Kill()
			}
		}


		if !ac.WatchMode { break }

		previousSecretsHash = currentSecretsHash

		time.Sleep(ac.WatchPollInterval)
	}

	<-done
	currentTempFactory.Cleanup()
	os.Exit(0)
}

// runSubcommand executes a command with arguments in the context
// of an environment populated with secret values.
func runSubCommand(command []string, env []string) (*exec.Cmd) {
	runner := exec.Command(command[0], command[1:]...)
	runner.Stdin = os.Stdin
	runner.Stdout = os.Stdout
	runner.Stderr = os.Stderr
	runner.Env = env

	return runner
}

func respondToError(out string, err error) {
	if err != nil {
		fmt.Println(out + ": " + err.Error())
		os.Exit(127)
	}
}

// formatForEnv returns a string in %k=%v format, where %k=namespace of the secret and
// %v=the secret value or path to a temporary file containing the secret
func formatForEnv(key string, value string, spec secretsyml.SecretSpec, tempFactory *TempFactory) string {
	if spec.IsFile() {
		fname := tempFactory.Push(value)
		value = fname
	}

	return fmt.Sprintf("%s=%s", key, value)
}

func joinEnv(env []string) string {
	return strings.Join(env, "\n") + "\n"
}

// scans arguments for the magic string; if found,
// creates a tempfile to which all the environment mappings are dumped
// and replaces the magic string with its path.
// Returns the path if so, returns an empty string otherwise.
func setupEnvFile(args []string, tmpl string, results []ProviderResult, tempFactory *TempFactory) string {
	resultsMap := make(map[string]string)
	for _, result := range results {
		resultsMap[result.Key] = result.Value
	}

	var envFile = ""

	for i, arg := range args {
		idx := strings.Index(arg, ENV_FILE_MAGIC)
		if idx >= 0 {
			if envFile == "" {
				var templateBuffer bytes.Buffer

				fmap := template.FuncMap{
					"singleQuoteEscape": singleQuoteEscape,
				}
				t := template.Must(template.New("summonenvtemplate").Funcs(fmap).Parse(tmpl))
				err := t.Execute(&templateBuffer, resultsMap)
				if err != nil {
					fmt.Println(err.Error())
					os.Exit(-1)
				}
				envFile = tempFactory.Push(templateBuffer.String())
			}
			args[i] = strings.Replace(arg, ENV_FILE_MAGIC, envFile, -1)
		}
	}

	return envFile
}

// convertSubsToMap converts the list of substitutions passed in via
// command line to a map
func convertSubsToMap(subs []string) map[string]string {
	out := make(map[string]string)
	for _, sub := range subs {
		s := strings.SplitN(sub, "=", 2)
		key, val := s[0], s[1]
		out[key] = val
	}
	return out
}

func returnStatusOfError(err error) (int, error) {
	if eerr, ok := err.(*exec.ExitError); ok {
		if ws, ok := eerr.Sys().(syscall.WaitStatus); ok {
			if ws.Exited() {
				return ws.ExitStatus(), nil
			}
		}
	}
	return 0, err
}
