package proxy

import (
	"log"
	"net"

	"github.com/yinqiwen/gsnova/common/event"
)

func NewAuthEvent() *event.AuthEvent {
	auth := &event.AuthEvent{}
	auth.User = GConf.Auth
	interfaces, err := net.Interfaces()
	if err != nil {
		log.Printf("No interface found:%v", err)
		return auth
	}
	for _, inter := range interfaces {
		auth.Mac = inter.HardwareAddr.String()
		if len(auth.Mac) > 0 {
			break
		}
	}
	return auth
}
