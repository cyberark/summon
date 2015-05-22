package main

func main() {
	if err := RunCLI(); err != nil {
		panic(err)
	}
}
