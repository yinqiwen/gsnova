package websocket

import (
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/yinqiwen/gsnova/common/mux"
	"github.com/yinqiwen/gsnova/remote"
	"github.com/yinqiwen/pmux"
)

var (
	upgrader = websocket.Upgrader{
		ReadBufferSize:  4096,
		WriteBufferSize: 4096,
	}
)

// handleWebsocket connection. Update to
func WebsocketInvoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", 405)
		return
	}

	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		//log.WithField("err", err).Println("Upgrading to websockets")
		http.Error(w, "Error Upgrading to websockets", 400)
		return
	}
	session, err := pmux.Server(&mux.WsConn{Conn: ws}, remote.InitialPMuxConfig())
	if nil != err {
		return
	}
	muxSession := &mux.ProxyMuxSession{Session: session}
	remote.ServProxyMuxSession(muxSession)
	//ws.WriteMessage(websocket.CloseMessage, []byte{})
}
