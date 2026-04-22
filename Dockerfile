FROM golang:1.26

WORKDIR /summon

ENV GOOS=linux
ENV GOARCH=amd64

COPY go.mod go.sum ./
COPY vendor/ vendor/

# hadolint ignore=DL3008
RUN apt-get update -y && \
    apt-get install -y --no-install-recommends bash git && \
    rm -rf /var/lib/apt/lists/* && \
    go install github.com/jstemmer/go-junit-report@latest && \
    go install github.com/afunix/gocov/gocov@latest && \
    go install github.com/AlekSi/gocov-xml@latest && \
    mkdir -p /summon/output

COPY . .

RUN go build -mod=vendor -o output/summon cmd/main.go
