package socks

import "fmt"

type proxiedAddr struct {
	net  string
	host string
	port int
}

func (a *proxiedAddr) Network() string {
	return a.net
}

func (a *proxiedAddr) String() string {
	return fmt.Sprintf("%s:%d", a.host, a.port)
}
