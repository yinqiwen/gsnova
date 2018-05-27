package protector

import (
	"time"
)

const (
	defaultDnsServer = "8.8.4.4"
	connectTimeOut   = 15 * time.Second
	readDeadline     = 15 * time.Second
	writeDeadline    = 15 * time.Second
	socketError      = -1
	dnsPort          = 53
)

type NetOptions struct {
	ReusePort   bool
	LocalAddr   string
	DialTimeout time.Duration
}

type Protect func(fileDescriptor int) error

var (
	currentProtect   Protect
	currentDnsServer string
)

func Configure(protect Protect, dnsServer string) {
	currentProtect = protect
	if dnsServer != "" {
		currentDnsServer = dnsServer
	} else {
		dnsServer = defaultDnsServer
	}
}

func SetDNSServer(server string) {
	if len(currentDnsServer) > 0 {
		currentDnsServer = server
	}
}

func init() {
	currentProtect = func(fd int) error {
		return nil
	}
}
