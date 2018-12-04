FROM golang:1.11-alpine

WORKDIR /summon

ENV GOOS=linux
ENV GOARCH=amd64

COPY go.mod go.sum ./

RUN apk add --no-cache bash \
                       git && \
    go mod download && \
    go get -u github.com/jstemmer/go-junit-report && \
    mkdir -p /summon/output

COPY . .

RUN go build -o output/summon cmd/main.go
