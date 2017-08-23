package quic

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"log"
	"math/big"

	quic "github.com/lucas-clemente/quic-go"
	"github.com/yinqiwen/gsnova/common/mux"
	"github.com/yinqiwen/gsnova/remote/channel"
)

// Setup a bare-bones TLS config for the server
func generateTLSConfig() *tls.Config {
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		panic(err)
	}
	template := x509.Certificate{SerialNumber: big.NewInt(1)}
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		panic(err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		panic(err)
	}
	return &tls.Config{Certificates: []tls.Certificate{tlsCert}}
}

func servQUIC(lp quic.Listener) {
	for {
		sess, err := lp.Accept()
		if nil != err {
			continue
		}
		muxSession := &mux.QUICMuxSession{Session: sess}
		go channel.ServProxyMuxSession(muxSession)
	}
	//ws.WriteMessage(websocket.CloseMessage, []byte{})
}

func StartQuicProxyServer(addr string) error {
	lp, err := quic.ListenAddr(addr, generateTLSConfig(), nil)
	if nil != err {
		log.Printf("[ERROR]Failed to listen QUIC address:%s with reason:%b", addr, err)
		return err
	}
	log.Printf("Listen on QUIC address:%s", addr)
	servQUIC(lp)
	return nil
}
