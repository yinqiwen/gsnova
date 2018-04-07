package helper

import (
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/easypki/pkg/certificate"
	"github.com/google/easypki/pkg/easypki"
	"github.com/google/easypki/pkg/store"
)

var pkiStore *easypki.EasyPKI
var ErrPKIStoreNotInit = errors.New("PKI store is not inited")
var caGenLock sync.Mutex

func CreateRootCA(dir string) error {
	os.Mkdir(dir, 0775)
	store.InitCADir(dir)
	pkiStore = &easypki.EasyPKI{Store: &store.Local{Root: dir}}
	if _, err := pkiStore.GetCA("Root"); nil == err {
		return nil
	}
	filename := "Root"
	subject := pkix.Name{CommonName: "GSNOVA Root CA"}
	template := &x509.Certificate{
		Subject:    subject,
		NotAfter:   time.Now().AddDate(0, 0, 36500),
		MaxPathLen: -1,
		IsCA:       true,
	}
	req := &easypki.Request{
		Name:                filename,
		Template:            template,
		IsClientCertificate: false,
		PrivateKeySize:      2048,
	}

	if err := pkiStore.Sign(nil, req); err != nil {
		return err
	}
	return nil
}

func GetCAByDomain(domain string) (*certificate.Bundle, error) {
	if nil == pkiStore {
		return nil, ErrPKIStoreNotInit
	}
	caGenLock.Lock()
	defer caGenLock.Unlock()
	//rootDomain, _ := publicsuffix.EffectiveTLDPlusOne(domain)
	rootDomain := domain
	bundle, err := pkiStore.GetBundle("Root", rootDomain)
	if nil == err {
		return bundle, nil
	}
	signer, err := pkiStore.GetCA("Root")
	if nil != err {
		return nil, err
	}
	filename := rootDomain
	subject := pkix.Name{CommonName: rootDomain}
	template := &x509.Certificate{
		Subject:    subject,
		NotAfter:   time.Now().AddDate(0, 0, 36500),
		MaxPathLen: -1,
		DNSNames:   []string{domain},
		//IsCA:       true,
	}
	req := &easypki.Request{
		Name:                filename,
		Template:            template,
		IsClientCertificate: false,
		PrivateKeySize:      2048,
	}
	if err = pkiStore.Sign(signer, req); err != nil {
		return nil, err
	}
	return pkiStore.GetBundle("Root", rootDomain)
}

func TLSConfig(domain string) (*tls.Config, error) {
	cfg := new(tls.Config)
	if strings.Contains(domain, ":") {
		domain = strings.Split(domain, ":")[0]
	}
	bundle, err := GetCAByDomain(domain)
	if nil != err {
		return nil, err
	}

	cert := tls.Certificate{}
	cert.Certificate = append(cert.Certificate, bundle.Cert.Raw)
	cert.PrivateKey = bundle.Key
	//cert.Leaf, _ = x509.ParseCertificate(bundle.Cert.Raw)
	cfg.Certificates = []tls.Certificate{cert}
	return cfg, nil
}
