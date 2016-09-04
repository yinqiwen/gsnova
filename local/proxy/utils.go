package proxy

import (
	"io/ioutil"
	"math/rand"
	"strings"
	"time"

	"github.com/yinqiwen/gsnova/common/event"
	"github.com/yinqiwen/gsnova/common/helper"
)

var currentDeviceId string

func getDeviceId() string {
	if len(currentDeviceId) > 0 {
		return currentDeviceId
	}
	deviceIdFile := proxyHome + "/.deviceid"
	storedId, err := ioutil.ReadFile(deviceIdFile)
	if nil == err && len(storedId) > 0 {
		currentDeviceId = string(storedId)
		currentDeviceId = strings.TrimSpace(currentDeviceId)
		if len(currentDeviceId) > 0 {
			return currentDeviceId
		}
	}
	currentDeviceId = helper.RandAsciiString(32)
	ioutil.WriteFile(deviceIdFile, []byte(currentDeviceId), 0660)
	return currentDeviceId
}

func NewAuthEvent() *event.AuthEvent {
	auth := &event.AuthEvent{}
	auth.User = GConf.Auth
	auth.Mac = getDeviceId()

	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	auth.SetId(uint32(r.Int31()))
	auth.Rand = []byte(helper.RandAsciiString(int(r.Int31n(128))))
	return auth
}

// func FillNOnce(auth *event.AuthEvent, nonceLen int) {
// 	auth.NOnce = make([]byte, nonceLen)
// 	io.ReadFull(rand.Reader, auth.NOnce)
// }
