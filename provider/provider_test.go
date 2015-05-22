package provider

import (
	. "github.com/smartystreets/goconvey/convey"
	"io/ioutil"
	"os"
	"testing"
)

func TestResolveProvider(t *testing.T) {
	Convey("Passing no provider should return an error", t, func() {
		_, err := ResolveProvider("")

		So(err, ShouldNotBeNil)
	})

	Convey("Passing the provider via CLI should return it without error", t, func() {
		expected := "/usr/bin/myprovider"
		provider, err := ResolveProvider(expected)

		So(provider, ShouldEqual, expected)
		So(err, ShouldBeNil)
	})

	Convey("Setting the provider via environment variable works", t, func() {
		expected := "/opt/providers/custom"
		os.Setenv("CAULDRON_PROVIDER", expected)
		provider, err := ResolveProvider("")
		os.Unsetenv("CAULDRON_PROVIDER")

		So(provider, ShouldEqual, expected)
		So(err, ShouldBeNil)
	})

	Convey("Given a provider path", t, func() {
		tempDir, _ := ioutil.TempDir("", "cauldrontest")
		defer os.RemoveAll(tempDir)
		DefaultProviderPath = tempDir

		Convey("If there is 1 executable, return it as the provider", func() {
			f, err := ioutil.TempFile(DefaultProviderPath, "")
			provider, err := ResolveProvider("")

			So(provider, ShouldEqual, f.Name())
			So(err, ShouldBeNil)
		})

		Convey("If there are > 1 executables, return an error to user", func() {
			// Create 2 exes in provider path
			ioutil.TempFile(DefaultProviderPath, "")
			ioutil.TempFile(DefaultProviderPath, "")
			_, err := ResolveProvider("")

			So(err, ShouldNotBeNil)
		})
	})
}

func TestCallProvider(t *testing.T) {
	Convey("When I call a provider", t, func() {
		Convey("If it returns exit code 0, return stdout", func() {
			arg := "provider.go"
			out, err := CallProvider("ls", arg)

			So(out, ShouldEqual, arg)
			So(err, ShouldBeNil)
		})
		Convey("If it returns exit code > 0, return stderr", func() {
			out, err := CallProvider("ls", "README.notafile")

			So(out, ShouldContainSubstring, "No such file or directory")
			So(err, ShouldNotBeNil)
		})
	})
}
