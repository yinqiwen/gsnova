package http2

import (
	"crypto/tls"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"sync"

	"golang.org/x/net/http2"

	"github.com/yinqiwen/gsnova/common/channel"
	"github.com/yinqiwen/gsnova/common/helper"
	"github.com/yinqiwen/gsnova/common/logger"
	"github.com/yinqiwen/gsnova/common/mux"
	//"github.com/yinqiwen/gsnova/remote"
)

type http2Stream struct {
	req     *http.Request
	rw      http.ResponseWriter
	rwLock  sync.Mutex
	closeCh chan struct{}
}

func (s *http2Stream) Read(b []byte) (n int, err error) {
	n, err = s.req.Body.Read(b)
	return
}

func (s *http2Stream) Write(b []byte) (n int, err error) {
	s.rwLock.Lock()
	defer s.rwLock.Unlock()
	if nil == s.rw {
		return 0, io.EOF
	}
	n, err = s.rw.Write(b)
	if nil != err {
		return n, err
	}
	if f, ok := s.rw.(http.Flusher); ok {
		f.Flush()
	}
	return n, nil
}

func (s *http2Stream) Close() (err error) {
	helper.AsyncNotify(s.closeCh)
	s.req.Body.Close()
	s.rwLock.Lock()
	s.rw = nil
	s.rwLock.Unlock()
	return nil
}

type http2Handler struct {
	session *mux.HTTP2MuxSession
}

func (ss *http2Handler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	//log.Printf("New HTTP2 stream %v", req)
	s := &http2Stream{}
	s.req = req
	s.rw = rw
	s.closeCh = make(chan struct{}, 1)
	rw.WriteHeader(200)
	err := ss.session.OfferStream(&helper.TimeoutReadWriteCloser{ReadWriteCloser: s})
	if nil != err {
		logger.Error("%v", err)
		return
	}
	<-s.closeCh
}

func servHTTP2(lp net.Listener, addr string, config *tls.Config) {
	for {
		conn, err := lp.Accept()
		if nil != err {
			continue
		}
		muxSession := mux.NewHTTP2ServerMuxSession(conn)
		go channel.ServProxyMuxSession(muxSession, nil, nil)
		server := &http.Server{
			Addr:      addr,
			TLSConfig: config,
		}
		http2Server := &http2.Server{
			MaxConcurrentStreams:         4096,
			PermitProhibitedCipherSuites: true,
		}
		opt := &http2.ServeConnOpts{}
		opt.BaseConfig = server
		opt.Handler = &http2Handler{session: muxSession}

		go func() {
			tlsconn := tls.Server(conn, config)
			err = tlsconn.Handshake()
			if nil != err {
				logger.Error("TLS handshake failed:%v", err)
				muxSession.Close()
				return
			}
			stateData, _ := json.MarshalIndent(tlsconn.ConnectionState(), "", "    ")
			logger.Notice("Recv conn state : %s", string(stateData))
			http2Server.ServeConn(tlsconn, opt)
			muxSession.Close()
		}()
	}
}

func StartHTTTP2ProxyServer(addr string, config *tls.Config) error {
	lp, err := net.Listen("tcp", addr)
	if nil != err {
		logger.Error("[ERROR]Failed to listen TCP address:%s with reason:%v", addr, err)
		return err
	}
	logger.Info("Listen on HTTP2 address:%s", addr)
	servHTTP2(lp, addr, config)
	return nil
}
