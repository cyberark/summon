FROM golang:1.25

WORKDIR /summon

ENV GOOS=linux
ENV GOARCH=amd64

COPY go.mod go.sum ./

RUN apt-get update -y && \
    apt-get install -y --no-install-recommends bash git && \
    go mod download && \
    go install github.com/jstemmer/go-junit-report@latest && \
    go install github.com/afunix/gocov/gocov@latest && \
    go install github.com/AlekSi/gocov-xml@latest && \
    mkdir -p /summon/output

COPY . .

RUN go build -o output/summon cmd/main.go
