package proxy

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/yinqiwen/gsnova/common/event"
	"github.com/yinqiwen/gsnova/common/helper"
	"github.com/yinqiwen/gsnova/common/logger"
	"github.com/yinqiwen/gsnova/local/hosts"
)

type Feature struct {
	MaxRequestBody int
}

type Proxy interface {
	Init() error
	Destory() error
	Features() Feature
	Serve(session *ProxySession, ev event.Event) error
}

var proxyTable = make(map[string]Proxy)

func RegisterProxy(name string, p Proxy) error {
	proxyTable[name] = p
	return nil
}

func getProxyByName(name string) Proxy {
	p, exist := proxyTable[name]
	if exist {
		return p
	}
	return nil
}

func Start(confdir string) error {
	clientConf := confdir + "/client.json"
	hostsConf := confdir + "/hosts.json"
	confdata, err := helper.ReadWithoutComment(clientConf, "//")
	if nil != err {
		//log.Println(err)
		return err
	}
	err = json.Unmarshal(confdata, &GConf)
	if nil != err {
		fmt.Printf("Failed to unmarshal json:%s to config for reason:%v", string(confdata), err)
		return err
	}
	err = GConf.init()
	if nil != err {
		return err
	}
	logger.InitLogger(GConf.Log)
	err = hosts.Init(hostsConf)
	if nil != err {
		return err
	}
	event.SetDefaultSecretKey(GConf.Encrypt.Method, GConf.Encrypt.Key)
	for name, p := range proxyTable {
		err := p.Init()
		if nil != err {
			log.Printf("Failed to init proxy:%s with error:%v", name, err)
		} else {
			log.Printf("Proxy:%s init success.", name)
		}
	}
	startLocalServers()
	return nil
}

func Stop() error {
	stopLocalServers()
	for name, p := range proxyTable {
		err := p.Destory()
		if nil != err {
			log.Printf("Failed to destroy proxy:%s with error:%v", name, err)
		} else {
			log.Printf("Proxy:%s destroy success.", name)
		}
	}
	return nil
}
