package paas

import (
	"common"
	"event"
	"log"
	"util"
)

type GAEHttpConnection struct {
	appid string
}

type GAE struct {
	conns map[string]*util.ListSelector
}

//func (conn *GAEConnection) WriteEvent(conn *SessionConnection, ev event.Event) error {
//	return nil
//}
//
//func (conn *GAEConnection) ReadEvent() (err error, ev event.Event) {
//	return nil, nil
//}

func (manager *GAE) GetRemoteConnection(ev event.Event) (RemoteConnection, error) {
	return nil, nil
}

func (manager *GAE) GetName() string {
	return GAE_NAME
}

func (manager *GAE) Init() error {
	common.Cfg.GetProperty("GAE", "")
	return nil
}
