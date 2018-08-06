#!/bin/bash -e

git fetch --tags  # jenkins does not do this automatically yet

docker-compose pull goreleaser

docker-compose run --rm goreleaser release --rm-dist --skip-validate
