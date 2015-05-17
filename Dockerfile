FROM tcnksm/gox:1.4.2-light

WORKDIR /goroot/src
RUN GOOS=windows GOARCH=amd64 CGO_ENABLED=0 ./make.bash --no-clean
RUN GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 ./make.bash --no-clean

ENV GOX_OS "darwin linux windows"
ENV GOX_ARCH "amd64"

CMD go get -d ./... && gox -verbose -output "pkg/{{.OS}}_{{.Arch}}/{{.Dir}}"
