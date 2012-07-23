package paas

import (
	"common"
	"net/http"
	"strings"
)

type SpacConfig struct {
	enableGoogleHttpDispatch  bool
	enableGoogleHttpsDispatch bool
	googleSites               []string
}

var spac *SpacConfig

var registedRemoteConnManager map[string]RemoteConnectionManager = make(map[string]RemoteConnectionManager)

func RegisteRemoteConnManager(connManager RemoteConnectionManager) {
	//connManager.Init()
	registedRemoteConnManager[connManager.GetName()] = connManager
}

func InitSpac() {
	spac = &SpacConfig{}
	spac.enableGoogleHttpDispatch, _ = common.Cfg.GetBoolProperty("SPAC", "GoogleHttpDispatchEnable")
	spac.enableGoogleHttpsDispatch, _ = common.Cfg.GetBoolProperty("SPAC", "GoogleHttpsDispatchEnable")
	if sites, exist := common.Cfg.GetProperty("SPAC", "GoogleSites"); exist {
		spac.googleSites = strings.Split(sites, "|")
	}
}

func SelectProxy(req *http.Request) (RemoteConnectionManager, bool) {
	host := strings.Split(req.Host, ":")[0]
	matched := false
	for _, pattern := range spac.googleSites {
		if strings.Contains(host, pattern) {
			matched = true
			break
		}
	}
	if matched {
		if strings.EqualFold(req.Method, "CONNECT") {
			if spac.enableGoogleHttpsDispatch {
				v, ok := registedRemoteConnManager[GOOGLE_NAME]
				return v, ok
			}
		} else {
			if spac.enableGoogleHttpDispatch {
				v, ok := registedRemoteConnManager[GOOGLE_NAME]
				return v, ok
			}
		}
	}
	v, ok := registedRemoteConnManager[GAE_NAME]
	return v, ok
}
