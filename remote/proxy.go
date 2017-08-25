package remote

import (
	"io"
	"log"
	"net"
	"time"

	"github.com/golang/snappy"
	"github.com/yinqiwen/gsnova/common/mux"
)

func handleProxyStream(stream mux.MuxStream, compresor string) {
	creq, err := mux.ReadConnectRequest(stream)
	defer stream.Close()
	if nil != err {
		stream.Close()
		log.Printf("[ERROR]:Failed to read connect request:%v", err)
		return
	}
	log.Printf("[%d]Start handle stream:%v with comprresor:%s", stream.StreamID(), creq, compresor)
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
	var streamReader io.Reader
	var streamWriter io.Writer
	streamReader = stream
	streamWriter = stream
	if compresor == mux.SnappyCompressor {
		streamReader = snappy.NewReader(stream)
		streamWriter = snappy.NewWriter(stream)
	}
	defer c.Close()
	go func() {
		io.Copy(c, streamReader)
	}()
	io.Copy(streamWriter, c)
	//n, err := io.Copy(stream, c)

}

func ServProxyMuxSession(session mux.MuxSession) error {
	isAuthed := false
	compressor := mux.SnappyCompressor
	for {
		stream, err := session.AcceptStream()
		if nil != err {
			//session.Close()
			log.Printf("Failed to accept stream with error:%v", err)
			return err
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
				return mux.ErrAuthFailed
			}
			switch auth.CompressMethod {
			case mux.SnappyCompressor:
			case mux.NoneCompressor:
			default:
				log.Printf("[ERROR]Invalid compressor:%s", auth.CompressMethod)
				session.Close()
				return mux.ErrAuthFailed
			}
			compressor = auth.CompressMethod
			isAuthed = true
			authRes := &mux.AuthResponse{Code: mux.AuthOK}
			mux.WriteMessage(stream, authRes)
			stream.Close()
			if tmp, ok := session.(*mux.ProxyMuxSession); ok {
				tmp.Session.ResetCryptoContext(auth.CipherMethod, auth.CipherCounter)
			}
			continue
		}
		go handleProxyStream(stream, compressor)
	}
}
