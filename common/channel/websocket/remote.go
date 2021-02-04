package websocket

import (
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/yinqiwen/gsnova/common/channel"
	"github.com/yinqiwen/gsnova/common/mux"
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

	//logger.Info("req headers: %v", r.Header, r.R)
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		//log.WithField("err", err).Println("Upgrading to websockets")
		http.Error(w, "Error Upgrading to websockets", 400)
		return
	}
	session, err := pmux.Server(&mux.WsConn{Conn: ws}, channel.InitialPMuxConfig(&channel.DefaultServerCipher))
	if nil != err {
		return
	}
	muxSession := &mux.ProxyMuxSession{Session: session}
	channel.ServProxyMuxSession(muxSession, nil, nil)
	//ws.WriteMessage(websocket.CloseMessage, []byte{})
}
