// +build mipsle mips

package helper

import (
	"crypto/tls"
	"errors"
)

var errNotSupportedForMIPS = errors.New("Not supported for MIPS")

func CreateRootCA(dir string) error {
	return errNotSupportedForMIPS
}

func TLSConfig(domain string) (*tls.Config, error) {
	return nil, errNotSupportedForMIPS
}
