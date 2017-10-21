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

func RegisterLocalChannelType(str string, p LocalChannel) error {
	rt := reflect.TypeOf(p)
	if rt.Kind() == reflect.Ptr {
		rt = rt.Elem()
	}
	LocalChannelTypeTable[str] = rt
	return nil
}

func AllowedSchema() []string {
	schames := []string{}
	for schema := range LocalChannelTypeTable {
		if schema != DirectChannelName {
			schames = append(schames, schema)
		}
	}
	sort.Strings(schames)
	return schames
}
