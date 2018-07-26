#!/bin/bash -e

docker-compose build --pull gosec-tester
docker-compose run --rm gosec-tester  # places gosec.junit.xml in output/
