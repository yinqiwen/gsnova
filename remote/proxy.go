package remote

import (
	"crypto/tls"
	"net/url"

	"github.com/yinqiwen/gsnova/common/helper"
	"github.com/yinqiwen/gsnova/common/logger"

	"github.com/yinqiwen/gsnova/common/channel/http2"
	"github.com/yinqiwen/gsnova/common/channel/kcp"
	"github.com/yinqiwen/gsnova/common/channel/quic"
	"github.com/yinqiwen/gsnova/common/channel/tcp"
)

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
	for _, lis := range ServerConf.Server {
		u, err := url.Parse(lis.Listen)
		if nil != err {
			logger.Error("Invalid listen url:%s for reason:%v", lis.Listen, err)
			continue
		}
		scheme := u.Scheme
		switch scheme {
		case "quic":
			{
				tlscfg, err := generateTLSConfig(lis.Cert, lis.Key)
				if nil != err {
					logger.Error("Failed to create TLS config by cert/key: %s/%s", lis.Cert, lis.Key)
				} else {
					go func() {
						quic.StartQuicProxyServer(u.Host, tlscfg)
					}()
				}
			}
		case "kcp":
			{
				go func() {
					kcp.StartKCPProxyServer(u.Host, &lis.KCParams)
				}()
			}
		case "tcp":
			{
				go func() {
					tcp.StartTcpProxyServer(u.Host)
				}()
			}
		case "tls":
			{
				tlscfg, err := generateTLSConfig(lis.Cert, lis.Key)
				if nil != err {
					logger.Error("Failed to create TLS config by cert/key: %s/%s", lis.Cert, lis.Key)
				} else {
					go func() {
						tcp.StartTLSProxyServer(u.Host, tlscfg)
					}()
				}
			}
		case "http":
			{
				go func() {
					startHTTPProxyServer(u.Host, "", "")
				}()
			}
		case "https":
			{
				go func() {
					startHTTPProxyServer(u.Host, lis.Cert, lis.Key)
				}()
			}
		case "http2":
			{
				tlscfg, err := generateTLSConfig(lis.Cert, lis.Key)
				if nil != err {
					logger.Error("Failed to create TLS config by cert/key: %s/%s", lis.Cert, lis.Key)
				} else {
					go func() {
						http2.StartHTTTP2ProxyServer(u.Host, tlscfg)
					}()
				}
			}
		default:
			logger.Error("Invalid listen scheme:%s in listen url:%s", scheme, lis.Listen)
		}
	}
}
