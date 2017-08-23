package main

import (
	"crypto/tls"
	"fmt"
	"io"
	"log"

	"github.com/yinqiwen/gotoolkit/ots"
	"github.com/yinqiwen/gsnova/remote"
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
	ots.RegisterHandler("vstat", dumpServerStat, 0, 0, "VStat                                 Dump server stat")
	err := ots.StartTroubleShootingServer(remote.ServerConf.AdminListen)
	if nil != err {
		log.Printf("Failed to start admin server with reason:%v", err)
		return
	}
	if len(remote.ServerConf.QUICListen) > 0 {
		go quic.StartQuicProxyServer(remote.ServerConf.QUICListen)
	}
	if len(remote.ServerConf.KCP.Listen) > 0 {
		go kcp.StartKCPProxyServer(remote.ServerConf.KCP.Listen)
	}
	if len(remote.ServerConf.TLS.Cert) > 0 {
		tlscfg := &tls.Config{}
		tlscfg.Certificates = make([]tls.Certificate, 1)
		tlscfg.Certificates[0], err = tls.LoadX509KeyPair(remote.ServerConf.TLS.Cert, remote.ServerConf.TLS.Key)
		if nil != err {
			log.Fatalf("Invalid cert/key for reason:%v", err)
			return
		}
		go tcp.StartTLSProxyServer(remote.ServerConf.TLS.Listen, tlscfg)
	}

	tcp.StartTcpProxyServer(remote.ServerConf.Listen)
	// go startLocalQUICProxyServer(remote.ServerConf.Listen)
	// startLocalProxyServer(remote.ServerConf.Listen)
}
