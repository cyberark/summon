FROM golang:1.27-alpine

WORKDIR /summon

ENV GOOS=linux
ENV GOARCH=amd64

COPY go.mod go.sum ./
COPY vendor/ vendor/

RUN apk add --no-cache bash git && \
    go install github.com/jstemmer/go-junit-report@latest && \
    go install github.com/afunix/gocov/gocov@latest && \
    go install github.com/AlekSi/gocov-xml@latest && \
    mkdir -p /summon/output

COPY . .

RUN go build -mod=vendor -o output/summon cmd/main.go
