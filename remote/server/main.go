package main

import (
	"crypto/tls"
	"fmt"
	"io"
	"log"

	"github.com/yinqiwen/gotoolkit/ots"
	"github.com/yinqiwen/gsnova/remote"
	"github.com/yinqiwen/gsnova/remote/channel/http"
	"github.com/yinqiwen/gsnova/remote/channel/kcp"
	"github.com/yinqiwen/gsnova/remote/channel/quic"
	"github.com/yinqiwen/gsnova/remote/channel/tcp"
)

func dumpServerStat(args []string, c io.Writer) error {
	fmt.Fprintf(c, "Version:    %s\n", remote.Version)
	// fmt.Fprintf(c, "NumSession:    %d\n", remote.GetSessionTableSize())
	// fmt.Fprintf(c, "NumEventQueue: %d\n", remote.GetEventQueueSize())
	// fmt.Fprintf(c, "NumActiveDynamicServer: %d\n", activeDynamicServerSize())
	// fmt.Fprintf(c, "NumRetiredDynamicServer: %d\n", retiredDynamicServerSize())
	// fmt.Fprintf(c, "TotalUserConn: %d\n", totalConn)
	return nil
}

func main() {
	if len(remote.ServerConf.AdminListen) > 0 {
		ots.RegisterHandler("vstat", dumpServerStat, 0, 0, "VStat                                 Dump server stat")
		err := ots.StartTroubleShootingServer(remote.ServerConf.AdminListen)
		if nil != err {
			log.Printf("Failed to start admin server with reason:%v", err)
		}
	}
	var serverDone []chan bool

	if len(remote.ServerConf.QUIC.Listen) > 0 {
		done := make(chan bool)
		serverDone = append(serverDone, done)
		go func() {
			quic.StartQuicProxyServer(remote.ServerConf.QUIC.Listen)
			done <- true
		}()
	}
	if len(remote.ServerConf.KCP.Listen) > 0 {
		done := make(chan bool)
		serverDone = append(serverDone, done)
		go func() {
			kcp.StartKCPProxyServer(remote.ServerConf.KCP.Listen)
			done <- true
		}()
	}
	if len(remote.ServerConf.TLS.Cert) > 0 && len(remote.ServerConf.TLS.Listen) > 0 {
		tlscfg := &tls.Config{}
		tlscfg.Certificates = make([]tls.Certificate, 1)
		var err error
		tlscfg.Certificates[0], err = tls.LoadX509KeyPair(remote.ServerConf.TLS.Cert, remote.ServerConf.TLS.Key)
		if nil != err {
			log.Fatalf("Invalid cert/key for reason:%v", err)
			return
		}
		done := make(chan bool)
		serverDone = append(serverDone, done)
		go func() {
			tcp.StartTLSProxyServer(remote.ServerConf.TLS.Listen, tlscfg)
			done <- true
		}()
	}
	if len(remote.ServerConf.HTTP.Listen) > 0 {
		done := make(chan bool)
		serverDone = append(serverDone, done)
		go func() {
			http.StartHTTPProxyServer(remote.ServerConf.HTTP.Listen)
			done <- true
		}()
	}
	if len(remote.ServerConf.TCP.Listen) > 0 {
		done := make(chan bool)
		serverDone = append(serverDone, done)
		go func() {
			tcp.StartTcpProxyServer(remote.ServerConf.TCP.Listen)
			done <- true
		}()
	}

	for _, done := range serverDone {
		<-done
	}
}
