FROM golang:1.19

WORKDIR /summon

ENV GOOS=linux
ENV GOARCH=amd64

COPY go.mod go.sum ./

RUN apt update -y && \
    apt install -y bash \
                   git && \
    go mod download && \
    go install github.com/jstemmer/go-junit-report@latest && \
    go install github.com/axw/gocov/gocov@latest && \
    go install github.com/AlekSi/gocov-xml@latest && \
    mkdir -p /summon/output

COPY . .

RUN go build -o output/summon cmd/main.go
