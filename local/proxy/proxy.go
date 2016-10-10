package proxy

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"reflect"
	"strings"

	"github.com/yinqiwen/gsnova/common/event"
	"github.com/yinqiwen/gsnova/common/helper"
	"github.com/yinqiwen/gsnova/common/logger"
	"github.com/yinqiwen/gsnova/local"
	"github.com/yinqiwen/gsnova/local/hosts"
)

var proxyHome string

type InternalEventMonitor func(code int, desc string) error

type Feature struct {
	MaxRequestBody int
}

type Proxy interface {
	Init(conf ProxyChannelConfig) error
	//Type() string
	Config() *ProxyChannelConfig
	Destory() error
	Features() Feature
	PrintStat(w io.Writer)
	Serve(session *ProxySession, ev event.Event) error
}

func init() {
	proxyHome = "."
}

var proxyTable = make(map[string]Proxy)
var proxyTypeTable map[string]reflect.Type = make(map[string]reflect.Type)

func RegisterProxyType(str string, p Proxy) error {
	rt := reflect.TypeOf(p)
	if rt.Kind() == reflect.Ptr {
		rt = rt.Elem()
	}
	proxyTypeTable[str] = rt
	//proxyTable[p.Name()] = p
	return nil
}

func getProxyByName(name string) Proxy {
	p, exist := proxyTable[name]
	if exist {
		return p
	}
	return nil
}

func Start(home string, monitor InternalEventMonitor) error {
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
	err = hosts.Init(hostsConf)
	if nil != err {
		log.Printf("Failed to init local hosts with reason:%v.", err)
	}
	event.SetDefaultSecretKey(GConf.Encrypt.Method, GConf.Encrypt.Key)
	proxyHome = home
	GConf.init()
	for _, conf := range GConf.Channel {
		conf.Type = strings.ToUpper(conf.Type)
		if t, ok := proxyTypeTable[conf.Type]; !ok {
			log.Printf("[ERROR]No registe proxy channel type for %s", conf.Type)
			continue
		} else {
			v := reflect.New(t)
			p := v.Interface().(Proxy)
			if !conf.Enable {
				continue
			}
			if 0 == conf.ConnsPerServer {
				conf.ConnsPerServer = 1
			}
			err = p.Init(conf)
			if nil != err {
				log.Printf("Proxy channel(%s):%s init failed with reason:%v", conf.Type, conf.Name, err)
			} else {
				log.Printf("Proxy channel(%s):%s init success", conf.Type, conf.Name)
				proxyTable[conf.Name] = p
			}
		}
	}

	logger.InitLogger(GConf.Log)
	log.Printf("Starting GSnova %s.", local.Version)
	go initDNS()
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
