#!/bin/sh

env
if [ -n "$TRAVIS_TAG"]; then 
    curl -O -L https://github.com/grammarly/rocker/releases/download/1.3.1/rocker-1.3.1_linux_amd64.tar.gz
    tar zxf rocker-0.2.2_linux_amd64.tar.gz
    ./rocker build -f Docker/server/Rockerfile --auth $DOCKER_USER:$DOCKER_PASS --push -var Version=$TRAVIS_TAG 
fi
