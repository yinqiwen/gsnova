package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"flag"
	"log"
	"math/big"
	"os"
	"time"
)

var hostName *string = flag.String("host", "Snova Root Fake CA", "Hostname to generate a certificate for")

func LoadClientCA(root *x509.Certificate, host string) {
	certfile := "twitter.com.cer"
	keyfile := "twitter.com.key"
	//if err != nil {
	priv, _ := rsa.GenerateKey(rand.Reader, 1024)
	now := time.Now()
	//var tmp *string = flag.String("host", host, "Hostname to generate a certificate for")
	template := x509.Certificate{
		SerialNumber: new(big.Int).SetInt64(0),
		Subject: pkix.Name{
			CommonName:   host,
			Organization: []string{"Snova Project"},
		},
		SignatureAlgorithm: x509.SHA1WithRSA,
		//AuthorityKeyId: []byte
		NotBefore: now.Add(-5 * time.Minute).UTC(),
		NotAfter:  now.AddDate(30, 0, 0).UTC(), // valid for 1 year.

		//SubjectKeyId: []byte{0x67, 0xf9, 0x4f, 0x18, 0x9f, 0x81, 0xf4, 0x82, 0x72, 0x12, 0x93, 0xcd, 0xfd, 0x6d, 0x92, 0xdb, 0xd6, 0x5e, 0x81, 0xd0},
		//KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, root, &priv.PublicKey, priv)
	if err != nil {
		log.Fatalf("Failed to create certificate: %s", err)
		return
	}

	certOut, err := os.Create(certfile)
	if err != nil {
		log.Fatalf("failed to open cert.pem for writing: %s", err)
		return
	}
	pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	certOut.Close()
	log.Print("written Fake-SnovaRoot-Certificate.cer\n")

	keyOut, err := os.OpenFile(keyfile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		log.Print("failed to open key.pem for writing:", err)
		return
	}
	pem.Encode(keyOut, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})
	keyOut.Close()
	//}

	//return tls.LoadX509KeyPair(certPath+host+".cer", certPath+host+".key")
}

func main() {
	flag.Parse()

	priv, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		log.Fatalf("failed to generate private key: %s", err)
		return
	}

	now := time.Now()

	template := x509.Certificate{
		SerialNumber: new(big.Int).SetInt64(0),
		Subject: pkix.Name{
			CommonName:   *hostName,
			Organization: []string{"Snova Project"},
		},
		SignatureAlgorithm:    x509.SHA1WithRSA,
		BasicConstraintsValid: true,
		IsCA:                  true,

		NotBefore: now.Add(-5 * time.Minute).UTC(),
		NotAfter:  now.AddDate(30, 0, 0).UTC(), // valid for 1 year.

		SubjectKeyId: []byte{0x67, 0xf9, 0x4f, 0x18, 0x9f, 0x81, 0xf4, 0x82, 0x72, 0x12, 0x93, 0xcd, 0xfd, 0x6d, 0x92, 0xdb, 0xd6, 0x5e, 0x81, 0xd0},
		//KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		log.Fatalf("Failed to create certificate: %s", err)
		return
	}

	certOut, err := os.Create("Fake-SnovaRoot-Certificate.cer")
	if err != nil {
		log.Fatalf("failed to open cert.pem for writing: %s", err)
		return
	}
	pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	certOut.Close()
	log.Print("written Fake-SnovaRoot-Certificate.cer\n")

	keyOut, err := os.OpenFile("Fake-SnovaRoot-Key.key", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		log.Print("failed to open key.pem for writing:", err)
		return
	}
	pem.Encode(keyOut, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})
	keyOut.Close()
	log.Print("written Fake-SnovaRoot-Key.key\n")

	LoadClientCA(&template, "twitter.com")
}
