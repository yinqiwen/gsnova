package upnp

import (
	"testing"
)

func TestXYZ(t *testing.T) {
	nat, err := Discover()
	if nil != err {
		t.Error("Failed to Discover:" + err.Error())
	} else {
	    //var ip string
		//err = nat.AddPortMapping("TCP", 48101, 48101, "GSnova", 3000)
		//err = nat.AddPortMapping("UDP", 48101, 48101, "GSnova", 3000)
		//err = nat.DeletePortMapping("TCP", 48101)
		//err = nat.AddPortMappingV2("udp", 48101, 48101, "GSnova", 3000)
		_, err = nat.GetExternalIPAddress()
		if nil != err {
			t.Error("Failed to add port mapping:" + err.Error())
		}
	}

}
