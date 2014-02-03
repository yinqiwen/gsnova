package common

import (
	"log"
	"net"
	//"net/url"
	"event"
	"util"
)

var Cfg *util.Ini
var CfgFile string

func InitConfig() error {
	cfg, err := util.LoadIniFile(CfgFile)
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
	//	if addr, exist := Cfg.GetProperty("LocalProxy", "Proxy"); exist {
	//		LocalProxy, _ = url.Parse(addr)
	//	}
	if enable, exist := Cfg.GetIntProperty("Misc", "DebugEnable"); exist {
		DebugEnable = (enable != 0)
	}
	if key, exist := Cfg.GetProperty("Misc", "RC4Key"); exist {
		RC4Key = key
	}
	event.SetRC4Key(RC4Key)
	return err
}
