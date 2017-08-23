FROM golang:1.8

RUN go get -u github.com/jstemmer/go-junit-report

RUN mkdir -p /go/src/github.com/cyberark/summon/output
WORKDIR /go/src/github.com/cyberark/summon

COPY . .

ENV GOOS=linux
ENV GOARCH=amd64

ENTRYPOINT ["/usr/local/go/bin/go"]
CMD ["build", "-v"]
