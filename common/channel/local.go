package channel

import (
	"reflect"
	"sort"

	"github.com/yinqiwen/gsnova/common/mux"
)

type LocalChannel interface {
	//PrintStat(w io.Writer)
	CreateMuxSession(server string, conf *ProxyChannelConfig) (mux.MuxSession, error)
	Features() FeatureSet
}

var LocalChannelTypeTable map[string]reflect.Type = make(map[string]reflect.Type)

const DirectChannelName = "direct"

var DirectSchemes = []string{
	DirectChannelName,
	"socks",
	"socks4",
	"socks5",
	"http_proxy",
}

func IsDirectScheme(scheme string) bool {
	for _, s := range DirectSchemes {
		if s == scheme {
			return true
		}
	}
	return false
}

func RegisterLocalChannelType(str string, p LocalChannel) error {
	rt := reflect.TypeOf(p)
	if rt.Kind() == reflect.Ptr {
		rt = rt.Elem()
	}
	LocalChannelTypeTable[str] = rt
	return nil
}

func AllowedSchema() []string {
	schemes := []string{}
	for scheme := range LocalChannelTypeTable {
		if !IsDirectScheme(scheme) {
			schemes = append(schemes, scheme)
		}
	}
	sort.Strings(schemes)
	return schemes
}
