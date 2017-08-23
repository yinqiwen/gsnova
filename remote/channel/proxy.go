package channel

import (
	"io"
	"log"
	"net"

	"github.com/yinqiwen/gsnova/common/mux"
	"github.com/yinqiwen/gsnova/remote"
	"github.com/yinqiwen/pmux"
)

func handleProxyStream(stream mux.MuxStream) {
	creq, err := mux.ReadConnectRequest(stream)
	defer stream.Close()
	if nil != err {
		stream.Close()
		log.Printf("[ERROR]:Failed to read connect request:%v", err)
		return
	}
	log.Printf("[%d]Start handle stream:%v", stream.(*mux.ProxyMuxStream).ReadWriteCloser.(*pmux.Stream).ID(), creq)
	c, err := net.Dial(creq.Network, creq.Addr)
	if nil != err {
		log.Printf("[ERROR]:Failed to connect %s:%v for reason:%v", creq.Network, creq.Addr, err)
		stream.Close()
		return
	}
	defer c.Close()
	go func() {
		io.Copy(c, stream)
	}()
	io.Copy(stream, c)
	//n, err := io.Copy(stream, c)

}

func ServProxyMuxSession(session mux.MuxSession) {
	isAuthed := false
	for {
		stream, err := session.AcceptStream()
		if nil != err {
			session.Close()
			return
		}
		if !isAuthed {
			auth, err := mux.ReadAuthRequest(stream)
			if nil != err {
				log.Printf("[ERROR]:Failed to read auth request:%v", err)
				continue
			}
			log.Printf("###Recv auth:%v", auth)
			if !remote.ServerConf.VerifyUser(auth.User) {
				log.Printf("[ERROR]Invalid user:%s", auth.User)
				session.Close()
				return
			}
			isAuthed = true
			authRes := &mux.AuthResponse{Code: mux.AuthOK}
			mux.WriteMessage(stream, authRes)
			stream.Close()
			if tmp, ok := session.(*mux.ProxyMuxSession); ok {
				tmp.Session.ResetCryptoContext(auth.CipherMethod, auth.CipherCounter)
			}
			continue
		}
		go handleProxyStream(stream)
	}
}
