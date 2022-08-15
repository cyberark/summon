module github.com/cyberark/summon

require (
	github.com/stretchr/testify v1.8.0
	github.com/urfave/cli v1.22.9
	golang.org/x/net v0.0.0-20220812174116-3211cb980234
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/cpuguy83/go-md2man/v2 v2.0.1 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/kr/pretty v0.3.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
)

go 1.17

replace gopkg.in/yaml.v2 v2.2.2 => gopkg.in/yaml.v2 v2.2.8

replace gopkg.in/yaml.v3 v3.0.0-20200313102051-9f266ea9e77c => gopkg.in/yaml.v3 v3.0.1
