package proxy

import (
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/yinqiwen/gsnova/common/helper"
	"github.com/yinqiwen/gsnova/common/logger"
	"github.com/yinqiwen/gsnova/common/mux"
	"github.com/yinqiwen/gsnova/local"
	"github.com/yinqiwen/gsnova/local/hosts"
	"github.com/yinqiwen/pmux"
)

var proxyHome string

type InternalEventMonitor func(code int, desc string) error

type Proxy interface {
	//Init(conf ProxyChannelConfig) error
	//PrintStat(w io.Writer)
	CreateMuxSession(server string, conf *ProxyChannelConfig) (mux.MuxSession, error)
	//Config() *ProxyChannelConfig
	//Serve(session *ProxySession, ev event.Event) error
}

func init() {
	proxyHome = "."
}

var proxyTypeTable map[string]reflect.Type = make(map[string]reflect.Type)
var clientConfName = "client.json"
var hostsConfName = "hosts.json"

func InitialPMuxConfig() *pmux.Config {
	cfg := pmux.DefaultConfig()
	cfg.CipherKey = []byte(GConf.Encrypt.Key)
	cfg.CipherMethod = mux.DefaultMuxCipherMethod
	cfg.CipherInitialCounter = mux.DefaultMuxInitialCipherCounter
	return cfg
}

type muxSessionHolder struct {
	creatTime  time.Time
	expireTime time.Time
	muxSession mux.MuxSession
	server     string
	p          Proxy
}

type proxyChannel struct {
	Conf         ProxyChannelConfig
	sessionMutex sync.Mutex
	sessions     map[*muxSessionHolder]bool
	proxies      map[Proxy]bool
}

func (ch *proxyChannel) createMuxSessionByProxy(p Proxy, server string) (mux.MuxSession, error) {
	session, err := p.CreateMuxSession(server, &ch.Conf)
	if nil == err {
		authStream, err := session.OpenStream()
		if nil != err {
			return nil, err
		}
		counter := uint64(helper.RandBetween(0, 100000))
		err = authStream.Auth(GConf.Auth, GConf.Encrypt.Method, counter)
		if nil != err {
			//authStream.Close()
			return nil, err
		}
		if psession, ok := session.(*mux.ProxyMuxSession); ok {
			err = psession.Session.ResetCryptoContext(GConf.Encrypt.Method, counter)
			if nil != err {
				log.Printf("[ERROR]Failed to reset cipher context with reason:%v", err)
				return nil, err
			}
		}

		holder := &muxSessionHolder{
			creatTime:  time.Now(),
			expireTime: time.Now().Add(30 * time.Minute),
			muxSession: session,
			server:     server,
		}
		ch.sessionMutex.Lock()
		ch.sessions[holder] = true
		ch.sessionMutex.Unlock()
	}
	return session, err
}

func (ch *proxyChannel) getMuxSession() (mux.MuxSession, error) {
	var session mux.MuxSession
	minStreamNum := -1
	var proxyServer string
	var p Proxy
	ch.sessionMutex.Lock()
	for holder := range ch.sessions {
		if len(proxyServer) == 0 {
			proxyServer = holder.server
			p = holder.p
		}
		if !holder.expireTime.IsZero() && holder.expireTime.Before(time.Now()) {
			if holder.muxSession.NumStreams() == 0 {
				holder.muxSession.Close()
				delete(ch.sessions, holder)
			}
			continue
		}
		if minStreamNum < 0 || holder.muxSession.NumStreams() < minStreamNum {
			minStreamNum = holder.muxSession.NumStreams()
			session = holder.muxSession
		}
	}
	ch.sessionMutex.Unlock()
	if nil == session {
		return ch.createMuxSessionByProxy(p, proxyServer)
	}
	return session, nil
}

var proxyChannelTable = make(map[string]*proxyChannel)
var proxyChannelMutex sync.Mutex

func RegisterProxyType(str string, p Proxy) error {
	rt := reflect.TypeOf(p)
	if rt.Kind() == reflect.Ptr {
		rt = rt.Elem()
	}
	proxyTypeTable[str] = rt
	return nil
}

func getMuxSessionByChannel(name string) (mux.MuxSession, error) {
	proxyChannelMutex.Lock()
	pch, exist := proxyChannelTable[name]
	proxyChannelMutex.Unlock()
	if !exist {
		return nil, fmt.Errorf("No proxy found to get mux session")
	}

	return pch.getMuxSession()
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

	proxyHome = home
	GConf.init()

	for _, conf := range GConf.Channel {
		if !conf.Enable {
			continue
		}
		channel := &proxyChannel{}
		channel.Conf = conf
		channel.sessions = make(map[*muxSessionHolder]bool)
		channel.proxies = make(map[Proxy]bool)
		success := false

		for _, server := range conf.ServerList {
			u, err := url.Parse(server)
			if nil != err {
				log.Printf("Invalid server url:%s with reason:%v", server, err)
				continue
			}
			schema := strings.ToLower(u.Scheme)
			if t, ok := proxyTypeTable[schema]; !ok {
				log.Printf("[ERROR]No registe proxy type for schema:%s", schema)
				continue
			} else {
				v := reflect.New(t)
				p := v.Interface().(Proxy)
				channel.proxies[p] = true
				for i := 0; i < conf.ConnsPerServer; i++ {
					_, err := channel.createMuxSessionByProxy(p, server)
					if nil != err {
						log.Printf("[ERROR]Failed to create mux session for %s:%d with reason:%v", server, i, err)
						break
					} else {
						success = true
					}
				}
			}
		}
		if success {
			log.Printf("Proxy channel:%s init success", conf.Name)
			proxyChannelTable[conf.Name] = channel
		} else {
			log.Printf("Proxy channel:%s init failed", conf.Name)
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
	proxyChannelMutex.Lock()
	for _, pch := range proxyChannelTable {
		for holder := range pch.sessions {
			if nil != holder {
				holder.muxSession.Close()
			}
		}
	}
	proxyChannelTable = make(map[string]*proxyChannel)
	proxyChannelMutex.Unlock()
	hosts.Clear()
	if nil != cnIPRange {
		cnIPRange.Clear()
	}
	return nil
}
