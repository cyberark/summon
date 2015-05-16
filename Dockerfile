FROM tcnksm/gox:1.4.2-light

RUN curl -sf -o gpm -L https://raw.githubusercontent.com/pote/gpm/v1.3.2/bin/gpm && \
chmod +x gpm && \
mv gpm /usr/local/bin

RUN curl -sf -o gpm-local -L https://raw.githubusercontent.com/technosophos/gpm-local/master/bin/gpm-local && \
chmod +x gpm-local && \
mv gpm-local /usr/local/bin

WORKDIR /goroot/src
RUN GOOS=windows GOARCH=amd64 CGO_ENABLED=0 ./make.bash --no-clean
RUN GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 ./make.bash --no-clean

ENV GOX_OS "darwin linux windows"
ENV GOX_ARCH "amd64"

CMD go get -d ./... && gox -verbose -output "pkg/{{.OS}}_{{.Arch}}/{{.Dir}}"
