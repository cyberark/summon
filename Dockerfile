FROM golang:1.15-alpine

WORKDIR /summon

ENV GOOS=linux
ENV GOARCH=amd64

COPY go.mod go.sum ./

RUN apk add --no-cache bash \
                       build-base \
                       docker-cli \
                       git && \
    go mod download && \
    go mod download && \
    go get -u github.com/jstemmer/go-junit-report && \
    go get -u github.com/axw/gocov/gocov && \
    go get -u github.com/AlekSi/gocov-xml && \
    mkdir -p /summon/output

COPY . .

RUN go build -o output/summon cmd/main.go
