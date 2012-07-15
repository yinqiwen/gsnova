package util

import (
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"log"
	"math/big"
	"math/rand"
	"time"
)

type PsuedoRandomReader struct{}

func (self *PsuedoRandomReader) Read(p []byte) (n int, err error) {
	for index := range p {
		p[index] = byte(rand.Intn(256))
	}
	return len(p), nil
}

func tlsConfig(host string) error {
	now := time.Now()
	tpl := x509.Certificate{
		SerialNumber:          new(big.Int).SetInt64(0),
		Subject:               pkix.Name{CommonName: host},
		NotBefore:             now.Add(-24 * time.Hour).UTC(),
		NotAfter:              now.AddDate(1, 0, 0).UTC(),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		MaxPathLen:            1,
		IsCA:                  true,
		SubjectKeyId:          []byte{1, 2, 3, 4},
		Version:               2,
	}
	priv, err := rsa.GenerateKey(new(PsuedoRandomReader), 512)
	if err != nil {
		return err
	}
	der, err := x509.CreateCertificate(new(PsuedoRandomReader), &tpl, &tpl, &priv.PublicKey, priv)
	if err != nil {
		return err
	}
	crt, err := x509.ParseCertificate(der)
	if err != nil {
		return err
	}
	opts := x509.VerifyOptions{DNSName: host, Roots: x509.NewCertPool()}
	opts.Roots.AddCert(crt)
	_, err = crt.Verify(opts)
	return err
}
