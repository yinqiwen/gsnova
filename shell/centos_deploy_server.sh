#!/bin/bash 

GO_RELEASE_FILE=go1.9.linux-amd64.tar.gz
GO_RELEASE_URL=https://storage.googleapis.com/golang/$GO_RELEASE_FILE
GSNOVA_SERVICE_DIR=gsnova_server

function  install_dependencies(){
    yum install git python-setuptools -y -q
    if [ $? -ne 0 ]; then
       echo "Failed to install git."
       exit 1
    fi
    easy_install supervisor
    if [ ! -f /etc/supervisord.conf ]; then
        echo_supervisord_conf > /etc/supervisord.conf
    fi
}
   

function  install_golang(){
    if [ -f $GO_RELEASE_FILE ]; then
        echo "File $GO_RELEASE_FILE exists."
    else
       echo "File $GO_RELEASE_FILE does not exist, dowload from $GO_RELEASE_URL"
       wget $GO_RELEASE_URL
       if [ $? -ne 0 ]; then
           echo "Download $GO_RELEASE_FILE failed."
           exit 1
        fi
    fi

    if [ ! -d "go" ]; then
        tar zxf $GO_RELEASE_FILE
        if [ $? -ne 0 ]; then
            echo "Extract $GO_RELEASE_FILE failed."
            exit 1
        fi
    fi
}

function  env_setting(){
    export GO_ROOT=$(pwd)/go
    export PATH=$GO_ROOT/bin:$PATH
    export GOPATH=$(pwd)/golang_projects
}

function  build_gsnova_server(){
    echo ">>>>> Syncing gsnova server code"
    go get -t -u -v github.com/yinqiwen/gsnova/remote/server
    if [ $? -ne 0 ]; then
       echo "Sync gsnova server code failed."
       return
    fi
    echo "<<<<< Done syncing gsnova server code"
    echo ">>>>> Building gsnova server"
    mkdir -p $GSNOVA_SERVICE_DIR; cd $GSNOVA_SERVICE_DIR
    go build  -v github.com/yinqiwen/gsnova/remote/server
    cp $GOPATH/src/github.com/yinqiwen/gsnova/server.json ./
    echo "<<<<< Done building gsnova server"
    echo "Please edit $GSNOVA_SERVICE_DIR/server.json before start gsnova_server."
    cd ..
}

function generate_supervise_conf(){
    cd $GSNOVA_SERVICE_DIR
    echo "[program:gsnova_server]
command=$(pwd)/server -conf $(pwd)/server.json
autostart=true
autorestart=true
startsecs=3
directory=$(pwd)
stderr_logfile=$(pwd)/gsnova_server_err.log
stdout_logfile=$(pwd)/gsnova_server.log" > gsnova_server_supervise.conf

   echo "Plase add $(pwd)/gsnova_server_supervise.conf into /etc/supervisord.conf include section first."
   echo "Then exec \"supervisord -c /etc/supervisord.conf\" to run gsnova server."
   #kill existing process
   kill -9 `cat .gsnova.pid 2>/dev/null` 2>/dev/null
}

install_dependencies
install_golang
env_setting
build_gsnova_server
generate_supervise_conf


