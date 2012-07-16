package paas

import (
	"net/http"
)

var registedRemoteConnManager map[string]RemoteConnectionManager = make(map[string]RemoteConnectionManager)

func RegisteRemoteConnManager(connManager RemoteConnectionManager) {
    //connManager.Init()
	registedRemoteConnManager[connManager.GetName()] = connManager
}

func SelectProxy(req *http.Request) (RemoteConnectionManager, bool) {
    v, ok := registedRemoteConnManager["GAE"]
	return v, ok
}
