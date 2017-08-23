package channel

import (
	"io"
	"log"
	"net"

	"github.com/yinqiwen/gsnova/common/mux"
	"github.com/yinqiwen/gsnova/remote"
)

func handleProxyStream(stream mux.MuxStream) {
	creq, err := mux.ReadConnectRequest(stream)
	if nil != err {
		stream.Close()
		log.Printf("[ERROR]:Failed to read connect request:%v", err)
		return
	}
	log.Printf("Start handle stream:%v", creq)
	c, err := net.Dial(creq.Network, creq.Addr)
	if nil != err {
		log.Printf("[ERROR]:Failed to connect %s:%v for reason:%v", creq.Network, creq.Addr, err)
		return
	}
	go func() {
		b := make([]byte, 8192)
		for {
			n, err := stream.Read(b)
			if n > 0 {
				//log.Printf("####Recv %s", string(b[0:n]))
				c.Write(b[0:n])
			}
			if nil != err {
				return
			}
		}
		//io.Copy(c, stream)
	}()

	io.Copy(stream, c)
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
