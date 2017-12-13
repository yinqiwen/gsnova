package remote

import (
	"crypto/tls"
	"fmt"
	"io"

	"github.com/yinqiwen/gsnova/common/channel"
	"github.com/yinqiwen/gsnova/common/helper"
	"github.com/yinqiwen/gsnova/common/logger"

	"github.com/yinqiwen/gsnova/common/channel/http2"
	"github.com/yinqiwen/gsnova/common/channel/kcp"
	"github.com/yinqiwen/gsnova/common/channel/quic"
	"github.com/yinqiwen/gsnova/common/channel/tcp"
)

func dumpServerStat(args []string, c io.Writer) error {
	fmt.Fprintf(c, "Version:    %s\n", channel.RemoteVersion)
	// fmt.Fprintf(c, "NumSession:    %d\n", remote.GetSessionTableSize())
	// fmt.Fprintf(c, "NumEventQueue: %d\n", remote.GetEventQueueSize())
	// fmt.Fprintf(c, "NumActiveDynamicServer: %d\n", activeDynamicServerSize())
	// fmt.Fprintf(c, "NumRetiredDynamicServer: %d\n", retiredDynamicServerSize())
	// fmt.Fprintf(c, "TotalUserConn: %d\n", totalConn)
	return nil
}

func generateTLSConfig(cert, key string) (*tls.Config, error) {
	if len(cert) > 0 {
		tlscfg := &tls.Config{}
		tlscfg.Certificates = make([]tls.Certificate, 1)
		var err error
		tlscfg.Certificates[0], err = tls.LoadX509KeyPair(cert, key)
		return tlscfg, err
	}
	return helper.GenerateTLSConfig(), nil
}

func StartRemoteProxy() {
	if len(ServerConf.QUIC.Listen) > 0 {
		tlscfg, err := generateTLSConfig(ServerConf.QUIC.Cert, ServerConf.QUIC.Key)
		if nil != err {
			logger.Error("Failed to create TLS config by cert/key: %s/%s", ServerConf.QUIC.Cert, ServerConf.QUIC.Key)
		} else {
			go func() {
				quic.StartQuicProxyServer(ServerConf.QUIC.Listen, tlscfg)
			}()
		}

	}
	if len(ServerConf.KCP.Listen) > 0 {
		go func() {
			kcp.StartKCPProxyServer(ServerConf.KCP.Listen, &ServerConf.KCP.KCPConfig)
		}()
	}
	if len(ServerConf.TLS.Listen) > 0 {
		tlscfg, err := generateTLSConfig(ServerConf.TLS.Cert, ServerConf.TLS.Key)
		if nil != err {
			logger.Error("Failed to create TLS config by cert/key: %s/%s", ServerConf.TLS.Cert, ServerConf.TLS.Key)
		} else {
			go func() {
				tcp.StartTLSProxyServer(ServerConf.TLS.Listen, tlscfg)
			}()
		}

	}
	if len(ServerConf.HTTP.Listen) > 0 {
		go func() {
			startHTTPProxyServer(ServerConf.HTTP.Listen)
		}()
	}
	if len(ServerConf.TCP.Listen) > 0 {
		go func() {
			tcp.StartTcpProxyServer(ServerConf.TCP.Listen)
		}()
	}
	if len(ServerConf.HTTP2.Listen) > 0 {
		tlscfg, err := generateTLSConfig(ServerConf.HTTP2.Cert, ServerConf.HTTP2.Key)
		if nil != err {
			logger.Error("Failed to create TLS config by cert/key: %s/%s", ServerConf.TLS.Cert, ServerConf.TLS.Key)
		} else {
			go func() {
				http2.StartHTTTP2ProxyServer(ServerConf.HTTP2.Listen, tlscfg)
			}()
		}
	}

}
