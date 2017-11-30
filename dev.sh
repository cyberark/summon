#!/bin/bash

echo "Running development environment"

docker-compose build --pull summon-builder

docker-compose run --rm -v $(pwd):/go/src/github.com/cyberark/summon/ --entrypoint bash summon-builder
