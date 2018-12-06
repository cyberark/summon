FROM golang:1.11-alpine

RUN apk add --no-cache bash \
                       build-base \
                       git \
                       libffi-dev \
                       ruby-bundler \
                       ruby-dev

# Install summon prerequisites
WORKDIR /summon
COPY go.mod go.sum ./
RUN go mod download

# Install test (Ruby) prerequisites
WORKDIR /summon/acceptance
COPY acceptance/Gemfile acceptance/Gemfile.lock ./
RUN bundle install

# Build summon
WORKDIR /summon
COPY . .
RUN go build -o /bin/summon cmd/main.go

# Run tests
WORKDIR /summon/acceptance
CMD ["cucumber"]
