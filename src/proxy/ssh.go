package proxy

import (
	"code.google.com/p/go.crypto/ssh"
	"event"
	"net"
	"net/http"
)

type SSHConnection struct {
}

func (conn *SSHConnection) Request(conn *SessionConnection, ev event.Event) (err error, res event.Event) {
	f := func(local, remote net.Conn) {
		n, err := io.Copy(remote, local)
		if nil != err {
			local.Close()
			remote.Close()
		}
	}

	switch ev.GetType() {
	case event.HTTP_REQUEST_EVENT_TYPE:
		req := ev.(*event.HTTPRequestEvent)
		if conn.Type == HTTPS_TUNNEL {

		} else {
		}
	default:
	}
	return nil, nil
}
func (conn *SSHConnection) GetConnectionManager() RemoteConnectionManager {

}
func (conn *SSHConnection) Close() error {

}

type SSH struct {
	//x *util.ListSelector
}

func (manager *SSH) RecycleRemoteConnection(conn RemoteConnection) {

}

func (manager *SSH) GetRemoteConnection(ev event.Event) (RemoteConnection, error) {
}

func (manager *SSH) GetName() string {
	return SSH_NAME
}
