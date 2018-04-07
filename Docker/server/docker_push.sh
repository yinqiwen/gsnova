#!/bin/sh

docker login -u "$DOCKER_USER" -p "$DOCKER_PASS"
docker build --build-arg GSNOVA_VER=${TRAVIS_TAG} -t gsnova/gsnova-server:${TRAVIS_TAG} ./Docker/server
docker push gsnova/gsnova-server:${TRAVIS_TAG}