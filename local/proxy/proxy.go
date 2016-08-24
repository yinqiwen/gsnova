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

var proxyHome string

var cnIPRange *IPRangeHolder

type Feature struct {
	MaxRequestBody int
}

type Proxy interface {
	Init() error
	Name() string
	Destory() error
	Features() Feature
	Serve(session *ProxySession, ev event.Event) error
}

func init() {
	proxyHome = "."
}

var proxyTable = make(map[string]Proxy)

func RegisterProxy(p Proxy) error {
	proxyTable[p.Name()] = p
	return nil
}

func getProxyByName(name string) Proxy {
	p, exist := proxyTable[name]
	if exist {
		return p
	}
	return nil
}

func Start(home string) error {
	clientConf := home + "/client.json"
	hostsConf := home + "/hosts.json"
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
	iprangeFile := home + "/apnic_cn.txt"
	cnIPRange, err = parseApnicIPFile(iprangeFile)
	if nil != err {
		fmt.Printf("Failed to parse iprange file:%s for reason:%v", iprangeFile, err)
		return err
	}

	err = GConf.init()
	if nil != err {
		return err
	}
	proxyHome = home
	logger.InitLogger(GConf.Log)
	err = hosts.Init(hostsConf)
	if nil != err {
		return err
	}
	if len(GConf.LocalDNS.Listen) > 0 {
		go startLocalDNS(GConf.LocalDNS.Listen)
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
	go startAdminServer()
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
	hosts.Clear()
	if nil != cnIPRange {
		cnIPRange.Clear()
	}
	return nil
}
