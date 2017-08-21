package proxy

import (
	"encoding/json"
	"fmt"
	"log"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/yinqiwen/gsnova/common/event"
	"github.com/yinqiwen/gsnova/common/helper"
	"github.com/yinqiwen/gsnova/common/logger"
	"github.com/yinqiwen/gsnova/local"
	"github.com/yinqiwen/gsnova/local/hosts"
)

var proxyHome string

type InternalEventMonitor func(code int, desc string) error

type Proxy interface {
	Init(conf ProxyChannelConfig) error
	//PrintStat(w io.Writer)
	CreateMuxSession(server string) (MuxSession, error)
	Config() *ProxyChannelConfig
	//Serve(session *ProxySession, ev event.Event) error
}

type BaseProxy struct {
	Conf ProxyChannelConfig
}

func (p *BaseProxy) Init(conf ProxyChannelConfig) error {
	p.Conf = conf
	return nil
}

func (p *BaseProxy) Config() *ProxyChannelConfig {
	return &p.Conf
}

func init() {
	proxyHome = "."
}

var proxyTable = make(map[string]Proxy)
var proxyTypeTable map[string]reflect.Type = make(map[string]reflect.Type)
var clientConfName = "client.json"
var hostsConfName = "hosts.json"

type muxSessionHolder struct {
	creatTime  time.Time
	expireTime time.Time
	muxSession MuxSession
	server     string
}

var proxyMuxSessionTable = make(map[Proxy]map[*muxSessionHolder]bool)
var proxyMuxSessionMutex sync.Mutex

func RegisterProxyType(str string, p Proxy) error {
	rt := reflect.TypeOf(p)
	if rt.Kind() == reflect.Ptr {
		rt = rt.Elem()
	}
	proxyTypeTable[str] = rt
	//proxyTable[p.Name()] = p
	return nil
}

func createMuxSessionByProxy(p Proxy, server string) (MuxSession, error) {
	proxyMuxSessionMutex.Lock()
	smap, exist := proxyMuxSessionTable[p]
	if !exist {
		smap = make(map[*muxSessionHolder]bool)
		proxyMuxSessionTable[p] = smap
	}
	proxyMuxSessionMutex.Unlock()
	session, err := p.CreateMuxSession(server)
	if nil == err {
		authStream, err := session.OpenStream()
		if nil != err {
			return nil, err
		}
		err = authStream.Auth(GConf.Auth)
		if nil != err {
			authStream.Close()
			return nil, err
		}
		authStream.Close()

		holder := &muxSessionHolder{
			creatTime:  time.Now(),
			expireTime: time.Now().Add(30 * time.Minute),
			muxSession: session,
			server:     server,
		}
		proxyMuxSessionMutex.Lock()
		smap[holder] = true
		proxyMuxSessionMutex.Unlock()
	}
	return session, err
}

func getMuxSessionByProxy(p Proxy) (MuxSession, error) {
	proxyMuxSessionMutex.Lock()
	smap, exist := proxyMuxSessionTable[p]
	if !exist {
		proxyMuxSessionMutex.Unlock()
		return nil, fmt.Errorf("No proxy found to get mux session")
	}
	var session MuxSession
	minStreamNum := -1
	var proxyServer string
	for holder := range smap {
		proxyServer = holder.server
		if !holder.expireTime.IsZero() && holder.expireTime.Before(time.Now()) {
			if holder.muxSession.NumStreams() == 0 {
				holder.muxSession.Close()
				delete(smap, holder)
			}
			continue
		}
		if minStreamNum < 0 || holder.muxSession.NumStreams() < minStreamNum {
			minStreamNum = holder.muxSession.NumStreams()
			session = holder.muxSession
		}
	}
	proxyMuxSessionMutex.Unlock()
	if nil == session {
		return createMuxSessionByProxy(p, proxyServer)
	}
	return session, nil
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
	}
	err := hosts.Init(conf)
	if nil != err {
		log.Printf("Failed to init local hosts with reason:%v.", err)
	}
	return err
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

			for _, server := range conf.ServerList {
				for i := 0; i < conf.ConnsPerServer; i++ {
					_, err := createMuxSessionByProxy(p, server)
					if nil != err {
						log.Printf("[ERROR]Failed to create mux session for %s:%d", server, i)
					}
				}
			}
			if len(proxyMuxSessionTable[p]) == 0 && !conf.IsDirect() {
				log.Printf("Proxy channel(%s):%s init failed", conf.Type, conf.Name)
				continue
			}
			log.Printf("Proxy channel(%s):%s init success", conf.Type, conf.Name)
			proxyTable[conf.Name] = p
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
	proxyMuxSessionMutex.Lock()
	for _, pmap := range proxyMuxSessionTable {
		for holder := range pmap {
			if nil != holder {
				holder.muxSession.Close()
			}
		}
	}
	proxyMuxSessionTable = make(map[Proxy]map[*muxSessionHolder]bool)
	proxyMuxSessionMutex.Unlock()
	hosts.Clear()
	if nil != cnIPRange {
		cnIPRange.Clear()
	}
	return nil
}
