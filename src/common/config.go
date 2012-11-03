package common

import (
	"log"
	"net"
	"net/url"
//	"time"
	"util"
)

var Cfg *util.Ini

func InitConfig() error {
	cfg, err := util.LoadIniFile(Home + Product + ".conf")
	Cfg = cfg
	if nil != err {
		log.Fatalf("Failed to load config file for reason:%s\n", err.Error())
	}
	if addr, exist := Cfg.GetProperty("LocalServer", "Listen"); exist {
		_, port, _ := net.SplitHostPort(addr)
		if len(port) > 0 {
			ProxyPort = port
		}
	}

//	if timeout, exist := Cfg.GetIntProperty("LocalServer", "KeepAliveTimeout"); exist {
//		KeepAliveTimeout = time.Duration(timeout)
//	}
	if addr, exist := Cfg.GetProperty("LocalProxy", "Proxy"); exist {
		LocalProxy, _ = url.Parse(addr)
	}
	if enable, exist := Cfg.GetIntProperty("Debug", "Enable"); exist {
		DebugEnable = (enable != 0)
	}
	//err = util.LoadHostMapping(Home + "hosts.conf")
	return err
}
