package fakecert

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	//"errors"

	"log"
	"math/big"
	"strings"
	"time"
)

var RootCert tls.Certificate
var X509RootCert *x509.Certificate
var RC4Key = "8976501f8451f03c5c4067b47882f2e5"

var cachedCertificates = make(map[string]tls.Certificate)

func randBigInt() (value *big.Int) {
	value, _ = rand.Int(rand.Reader, big.NewInt(0x7FFFFFFFFFFFFFFF))
	return
}

func randBytes() (bytes []byte) {
	bytes = make([]byte, 20)
	rand.Read(bytes)
	return
}

func init() {
	cert := "Fake-ACRoot-Certificate.cer"
	key := "Fake-ACRoot-Key.pem"
	root_cert, err := tls.LoadX509KeyPair(cert, key)
	if nil == err {
		RootCert = root_cert
		X509RootCert, err = x509.ParseCertificate(root_cert.Certificate[0])
		return
	}
	log.Printf("###Failed to load root cert:%v", err)
}

func TLSConfig(host string) (*tls.Config, error) {
	cfg := new(tls.Config)
	if strings.Contains(host, ":") {
		host = strings.Split(host, ":")[0]
	}
	cert, err := getTLSCert(host)
	if nil != err {
		log.Printf("Failed to get tls cert:%v", err)
		return nil, err
	}
	cfg.Certificates = []tls.Certificate{cert}
	//cfg.BuildNameToCertificate()
	return cfg, nil
}

func getTLSCert(host string) (tls.Certificate, error) {
	var tls_cer tls.Certificate
	if cert, exist := cachedCertificates[host]; exist {
		return cert, nil
	}

	priv, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		return tls_cer, err
	}
	serial := randBigInt()
	keyId := randBytes()
	template := x509.Certificate{
		Subject: pkix.Name{
			CommonName: host,
		},
		Issuer: pkix.Name{
			CommonName: "GSnova Root Fake CA",
		},
		SerialNumber:   serial,
		SubjectKeyId:   keyId,
		AuthorityKeyId: X509RootCert.AuthorityKeyId,
		NotBefore:      time.Now().Add(-5 * time.Minute).UTC(),
		NotAfter:       time.Now().AddDate(12, 0, 0).UTC(),
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, X509RootCert, &priv.PublicKey, RootCert.PrivateKey)
	if err != nil {
		return tls_cer, err
	}
	crt, err := x509.ParseCertificate(derBytes)
	if err != nil {
		return tls_cer, err
	}
	cBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: crt.Raw})
	kBytes := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})
	tls_cer, err = tls.X509KeyPair(cBytes, kBytes)
	if nil == err {
		cachedCertificates[host] = tls_cer
	}
	return tls_cer, err
}
