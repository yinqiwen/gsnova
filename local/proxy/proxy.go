package proxy

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"reflect"
	"strings"

	"github.com/fsnotify/fsnotify"
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
var clientConfName = "client.json"
var hostsConfName = "hosts.json"

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

func loadConf(conf string) error {
	if strings.HasSuffix(conf, clientConfName) {
		confdata, err := helper.ReadWithoutComment(conf, "//")
		if nil != err {
			log.Println(err)
		}
		err = json.Unmarshal(confdata, &GConf)
		if nil != err {
			fmt.Printf("Failed to unmarshal json:%s to config for reason:%v", string(confdata), err)
		}
		return err
	} else {
		err := hosts.Init(conf)
		if nil != err {
			log.Printf("Failed to init local hosts with reason:%v.", err)
		}
		return err
	}
}

func watchConf(watcher *fsnotify.Watcher) {
	for {
		select {
		case event := <-watcher.Events:
			//log.Println("event:", event)
			if event.Op&fsnotify.Write == fsnotify.Write {
				log.Println("modified file:", event.Name)
				loadConf(event.Name)
			}
		case err := <-watcher.Errors:
			log.Println("error:", err)
		}
	}
}

func Start(home string, monitor InternalEventMonitor) error {
	confWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
		return err
	}
	clientConf := home + "/" + clientConfName
	hostsConf := home + "/" + hostsConfName
	confWatcher.Add(clientConf)
	confWatcher.Add(hostsConf)
	go watchConf(confWatcher)
	err = loadConf(clientConf)
	if nil != err {
		//log.Println(err)
		return err
	}
	loadConf(hostsConf)

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
