#!/bin/bash 

GSNOVA_SERVICE_DIR=gsnova_server
GSNOVA_VER=latest
GSNOVA_FILE=gsnova_linux_amd64-$GSNOVA_VER.tar.bz2
GSNOVA_RELEASE_URL=https://github.com/yinqiwen/gsnova/releases/download/$GSNOVA_VER/$GSNOVA_FILE    

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
   


function  download_gsnova_server(){
    mkdir -p $GSNOVA_SERVICE_DIR; cd $GSNOVA_SERVICE_DIR
    echo ">>>>> Syncing gsnova server"
    wget $GSNOVA_RELEASE_URL -O $GSNOVA_FILE
    if [ $? -ne 0 ]; then
       echo "Sync gsnova server code failed."
       return
    fi
    echo "<<<<< Done syncing gsnova server"
    echo ">>>>> Building gsnova server"
    
    tar jxf $GSNOVA_FILE
    if [ ! -f myserver.json ]; then
        cp ./server.json ./myserver.json
    fi
    
    echo "<<<<< Done building gsnova server"
    echo "Please edit $GSNOVA_SERVICE_DIR/myserver.json before start gsnova_server."
    cd ..
}

function generate_supervise_conf(){
    cd $GSNOVA_SERVICE_DIR
    echo "[program:gsnova_server]
command=$(pwd)/gsnova -server -conf $(pwd)/myserver.json -admin :60000
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
#install_golang
#env_setting
download_gsnova_server
generate_supervise_conf


