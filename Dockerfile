FROM tcnksm/gox:1.4.2-light

WORKDIR /goroot/src
RUN GOOS=windows GOARCH=amd64 CGO_ENABLED=0 ./make.bash --no-clean
RUN GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 ./make.bash --no-clean
