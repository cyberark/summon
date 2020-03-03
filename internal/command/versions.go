package command

import (
	"bytes"
	"fmt"
	//"io/ioutil"
	//"os"
	"os/exec"
	"path"

	"github.com/cyberark/summon/pkg/summon"
	"github.com/cyberark/summon/provider"
)
//Create the AllVersions string to include versions of all the providers
var DefaultPath = provider.DefaultPath

//Run `$provider --version`(from DefaultPath), for each provider in the list of providers
//Then appends the responded versions to string and returns it
func GetVersions() string{
	providers := getProviderFileNames()

	var providerVersions bytes.Buffer
	providerVersions.WriteString("\n")
	
	for _, provider := range providers {
		version, err := exec.Command(path.Join(DefaultPath, provider), "--version").Output()
		if err != nil {
			providerVersions.WriteString(provider+" version "+"uknown")
			fmt.Println("ERROR: "+provider, err, "running command :"+DefaultPath+"/"+ provider, "--version")	//debug
			continue
		}

		providerVersions.WriteString(provider+" version "+string(version[0:len(version)]))
	}
	return summon.VERSION + providerVersions.String()
}

//Create slice of all file names in the default path
func getProviderFileNames() []string{
	// files, err := ioutil.ReadDir(DefaultPath)
    // if err != nil {
    //     fmt.Println(err.Error())
	// 	os.Exit(127)
	// }
	// names := make([]string, len(files))
	// for i, file := range files {
    //     names[i] = file.Name()
	// }
	
	//CALLING RESOLVE TO MAKE SURE PROVIDERS IS PROPERLY FILLED. WORRIED ABOUT THAT CAUSING ISSUES
	provider.Resolve("")
    return provider.Providers
}
