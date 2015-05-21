package main

import (
    "fmt"
    "os"
    "errors"
    "os/exec"
    "strings"
    "syscall"
    "github.com/conjurinc/cauldron"
)

//
// Example cauldron backend that fetches secrets from the OSX keychain.
//
// Although there are go bindings for the OSX keychain API, they are of uncertain
// stability, so we simply shell out to the 'security' command.
//
// To try this out, you can add secrets like this:
// $ security add-generic-password -s cauldron -a "my/secret/path" -w "my secret value"


// The OSX Keychain uses a model similar to .netrc: secrets are stored as 
// (service, account, secret) tuples, so we need to provide a 'service': 
// we use 'cauldron' by default, but you can specify a different service with
// the CAULDRON_KEYCHAIN_SERVICE_NAME environment variable.
const SERVICE_NAME_KEY = "CAULDRON_KEYCHAIN_SERVICE_NAME"
const DEFAULT_SERVICE_NAME = "cauldron"

func serviceName() string {
    env := os.Getenv(SERVICE_NAME_KEY)
    if len(env) != 0 {
        return env
    }
    return DEFAULT_SERVICE_NAME
}


// Figuring out what went wrong is a bit hairy.  If we're run on a platform
// that doesn't have the 'security' command, cmd.ProcessState will be nil.
//  
// We understand two error codes (found via trial and error):
// 0x2C:  the secret wasn't found
// 0x80: the user cancelled the unlock keychain dialog (possibly other conditions?)
//
// In other cases we defer to (* ProcessStatus) String()

const STATUS_NOT_FOUND = 0x2C
const STATUS_CANCELLED = 0x80

func getCmdError(cmd *exec.Cmd, service, account string) error {
    ps := cmd.ProcessState

    if ps == nil {
        // Command 'security' wasn't found, are you even a mac bro?
        return errors.New("Command 'security' not found.  Are on OSX? Because this only works on OSX.")
    }

    var msg string

    if ws, ok := ps.Sys().(syscall.WaitStatus); ok {
        switch ws.ExitStatus() {
            case STATUS_NOT_FOUND:
                msg = "secret not found"
            case STATUS_CANCELLED:
                msg = "keychain unlock failed"
            default:
                msg = ps.String()
        }

        return fmt.Errorf("Error fetching secret '%q' from service '%q': %q",
            account,service,msg)
    }

    // getting here represents a rather serious bug -- just panic
    panic("unreachable")
}


// Here is where we shell out the 'security' command to get a secret.
// 
// We return the output of the command, less a trailing newline, if 
// it succeeds, and the result of getCmdError if it doesn't.
func findGenericPassword(service, account string) (string, error) {
    cmd := exec.Command("security",
        "find-generic-password", // subcommand
        "-a", account,
        "-s", service,
        "-w")

    out, err := cmd.Output()

    if err != nil {
        return "", getCmdError(cmd, service, account)
    }

    return strings.TrimSuffix(string(out), "\n"), nil
}



// This implements the backend itself.  It calls findGenericPassword
// with serviceName() and the secret path.
func FetchFromKeychain(secret string) (string, error) {
    return findGenericPassword(serviceName(), secret)
}



func main(){
    // create and run a 'Cauldron' instance
    c := cauldron.NewCLI("keychain", "0.1.0", FetchFromKeychain)

    err := c.Start()

    if err != nil {
        // panic it to facilitate debugging
        panic(err)
    }
}

