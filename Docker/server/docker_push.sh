#!/bin/sh

echo "$DOCKER_PASS" | docker login -u "$DOCKER_USER" --password-stdin
docker build --build-arg GSNOVA_VER=${TRAVIS_TAG} -t gsnova/gsnova-server:${TRAVIS_TAG} ./Docker/server
docker push gsnova/gsnova-server:${TRAVIS_TAG}