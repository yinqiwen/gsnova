package local

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/yinqiwen/gotoolkit/gfwlist"
	"github.com/yinqiwen/gsnova/common/channel"
	"github.com/yinqiwen/gsnova/common/dns"
	"github.com/yinqiwen/gsnova/common/helper"
	"github.com/yinqiwen/gsnova/common/hosts"
	"github.com/yinqiwen/gsnova/common/logger"
)

var proxyHome string

var localGFWList atomic.Value
var fetchGFWListRunning bool

func init() {
	proxyHome = "."
}

var clientConfName = "client.json"
var hostsConfName = "hosts.json"

func loadClientConf(conf string) error {
	confdata, err := helper.ReadWithoutComment(conf, "//")
	if nil != err {
		logger.Error("Failed to load conf:%s with reason:%v", conf, err)
	}
	GConf = LocalConfig{}
	err = json.Unmarshal(confdata, &GConf)
	if nil != err {
		logger.Error("Failed to unmarshal json:%s to config for reason:%v", string(confdata), err)
	}
	return GConf.init()
}

func loadHostsConf(conf string) error {
	err := hosts.Init(conf)
	if nil != err {
		logger.Error("Failed to init local hosts with reason:%v.", err)
	}
	return err
}

func watchConf(watcher *fsnotify.Watcher) {
	for {
		select {
		case event := <-watcher.Events:
			logger.Debug("fsnotify event:%v", event)
			if (event.Op & fsnotify.Write) == fsnotify.Write {
				loadClientConf(event.Name)
			}
		case err := <-watcher.Errors:
			logger.Error("error:%v", err)
		}
	}
}

type ProxyOptions struct {
	Config    string
	Hosts     string
	CNIP      string
	Home      string
	WatchConf bool
}

func getGFWList() *gfwlist.GFWList {
	v := localGFWList.Load()
	if nil != v {
		return v.(*gfwlist.GFWList)
	}
	return nil
}

func loadGFWList(hc *http.Client) error {
	resp, err := hc.Get(GConf.GFWList.URL)
	if nil != err {
		logger.Error("Failed to fetch GFWList")
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		logger.Error("Failed to fetch GFWList with res:%v", resp)
		return err
	}
	body, err := ioutil.ReadAll(resp.Body)
	if nil != err {
		logger.Error("Failed to read GFWList with err:%v", err)
		return err
	}
	gfw, err := gfwlist.NewFromString(string(body), true)
	if nil != err {
		logger.Error("Invalid GFWList content:%v", err)
		return err
	}
	for _, rule := range GConf.GFWList.UserRule {
		gfw.Add(rule)
	}
	logger.Info("GFWList sync success.")
	localGFWList.Store(gfw)
	return nil
}

func initGFWList() {
	if fetchGFWListRunning {
		return
	}
	fetchGFWListRunning = true
	if len(GConf.GFWList.URL) > 0 {
		hc, _ := channel.NewHTTPClient(&channel.ProxyChannelConfig{Proxy: GConf.GFWList.Proxy}, "http")
		for {
			err := loadGFWList(hc)
			var nextRefreshTime time.Duration
			if nil == err {
				if GConf.GFWList.RefershPeriodMiniutes <= 0 {
					GConf.GFWList.RefershPeriodMiniutes = 1440
				}
				nextRefreshTime = time.Duration(GConf.GFWList.RefershPeriodMiniutes) * time.Minute
			} else {
				nextRefreshTime = 5 * time.Second
			}
			logger.Info("Refresh GFWList after %v.", nextRefreshTime)
			time.Sleep(nextRefreshTime)
		}
	}
}

func StartProxy() error {
	GConf.init()
	logger.InitLogger(GConf.Log)
	channel.SetDefaultMuxConfig(GConf.Mux)

	channel.UPNPExposePort = GConf.UPNPExposePort
	if GConf.TransparentMark > 0 {
		enableTransparentSocketMark(GConf.TransparentMark)
	}
	dns.Init(&GConf.LocalDNS)
	go initGFWList()

	logger.Notice("Allowed proxy channel with schema:%v", channel.AllowedSchema())
	singalCh := make(chan bool, len(GConf.Channel))
	channelCount := 0
	for _, conf := range GConf.Channel {
		if !conf.Enable {
			continue
		}
		channel := channel.NewProxyChannel(&conf)
		channel.Conf = conf
		channelCount++
		go func() {
			channel.Init(true)
			singalCh <- true
		}()
	}
	for i := 0; i < channelCount; i++ {
		<-singalCh
	}

	err := helper.CreateRootCA(proxyHome + "/MITM")
	if nil != err {
		logger.Notice("Create MITM Root CA:%v", err)
	}

	logger.Info("Started GSnova %s.", channel.Version)

	go startAdminServer()
	startLocalServers()
	return nil
}

func Start(options ProxyOptions) error {
	clientConf := options.Config
	hostsConf := options.Hosts
	proxyHome = options.Home

	if options.WatchConf {
		confWatcher, err := fsnotify.NewWatcher()
		if err != nil {
			logger.Fatal("%v", err)
			return err
		}
		confWatcher.Add(clientConf)
		//confWatcher.Add(hostsConf)
		go watchConf(confWatcher)
	}

	if len(clientConf) > 0 {
		err := loadClientConf(clientConf)
		if nil != err {
			//log.Println(err)
			return err
		}
	} else {
		// if len(GConf.Proxy) == 0 {
		// 	return errors.New("Can NOT start proxy without any config")
		// }
	}
	GConf.LocalDNS.CNIPSet = options.CNIP
	channel.SetDefaultProxyLimitConfig(GConf.ProxyLimit)
	loadHostsConf(hostsConf)
	return StartProxy()
}

func Stop() error {
	stopLocalServers()
	channel.StopLocalChannels()
	hosts.Clear()
	return nil
}
