#!/bin/bash

app="summon"

rm -rf pkg

docker build -t summon/build .

projectpath="/goroot/src/github.com/conjurinc/${app}"
buildcmd='GOX_OS="darwin linux windows" GOX_ARCH="amd64" gox -verbose -output "pkg/{{.OS}}_{{.Arch}}/{{.Dir}}"'

docker run --rm \
-v "$(pwd)":"${projectpath}" \
-w "${projectpath}" \
-e "GOPATH=/goroot/src/github.com/conjurinc/${app}/Godeps/_workspace:/goroot" \
summon/build \
bash -c "${buildcmd}"
