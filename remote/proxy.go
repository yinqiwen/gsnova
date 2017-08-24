package remote

import (
	"io"
	"log"
	"net"
	"time"

	"github.com/yinqiwen/gsnova/common/mux"
)

func handleProxyStream(stream mux.MuxStream) {
	creq, err := mux.ReadConnectRequest(stream)
	defer stream.Close()
	if nil != err {
		stream.Close()
		log.Printf("[ERROR]:Failed to read connect request:%v", err)
		return
	}
	log.Printf("[%d]Start handle stream:%v", stream.StreamID(), creq)
	timeout := ServerConf.DialTimeout
	if timeout == 0 {
		timeout = 10
	}
	c, err := net.DialTimeout(creq.Network, creq.Addr, time.Duration(timeout)*time.Second)
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
			if !ServerConf.VerifyUser(auth.User) {
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
