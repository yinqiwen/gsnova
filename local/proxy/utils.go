package proxy

import (
	"log"
	"math/rand"
	"net"
	"time"

	"github.com/yinqiwen/gsnova/common/event"
)

var currentMacAddress string

func NewAuthEvent() *event.AuthEvent {
	auth := &event.AuthEvent{}
	auth.User = GConf.Auth
	if len(currentMacAddress) == 0 {
		interfaces, err := net.Interfaces()
		if err != nil {
			log.Printf("No interface found:%v", err)
			return auth
		}
		for _, inter := range interfaces {
			currentMacAddress = inter.HardwareAddr.String()
			if len(currentMacAddress) > 0 {
				break
			}
		}
	}
	auth.Mac = currentMacAddress
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	auth.SetId(uint32(r.Int31()))
	return auth
}

// func FillNOnce(auth *event.AuthEvent, nonceLen int) {
// 	auth.NOnce = make([]byte, nonceLen)
// 	io.ReadFull(rand.Reader, auth.NOnce)
// }
