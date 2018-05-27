package quic

import (
	"crypto/tls"

	quic "github.com/lucas-clemente/quic-go"
	"github.com/yinqiwen/gsnova/common/channel"
	"github.com/yinqiwen/gsnova/common/logger"
	"github.com/yinqiwen/gsnova/common/mux"
)

func servQUIC(lp quic.Listener) {
	for {
		sess, err := lp.Accept()
		if nil != err {
			continue
		}
		muxSession := &mux.QUICMuxSession{Session: sess}
		go channel.ServProxyMuxSession(muxSession, nil, nil)
	}
	//ws.WriteMessage(websocket.CloseMessage, []byte{})
}

func StartQuicProxyServer(addr string, config *tls.Config) error {
	lp, err := quic.ListenAddr(addr, config, nil)
	if nil != err {
		logger.Error("[ERROR]Failed to listen QUIC address:%s with reason:%v", addr, err)
		return err
	}
	logger.Info("Listen on QUIC address:%s", addr)
	servQUIC(lp)
	return nil
}
