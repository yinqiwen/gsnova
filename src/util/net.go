package util

import (
	"fmt"
	"math"
	"net"
	"os"
	"strconv"
	"strings"
)

func Long2IPv4(i int64) string {
	return fmt.Sprintf("%d.%d.%d.%d", (i>>24)&0xFF, (i>>16)&0xFF, (i>>8)&0xFF, i&0xFF)
}

func IPv42Int(ip string) int64 {
	addrArray := strings.Split(ip, ".")
	var num int64
	num = 0
	for i := 0; i < len(addrArray); i++ {
		power := 3 - i
		v, _ := strconv.Atoi(addrArray[i])
		num += (int64(v) % 256 * int64(math.Pow(float64(256), float64(power))))
	}
	return num
}

func IsPrivateIP(ip string) bool {
	value := IPv42Int(ip)
	if (value >= 0x0A000000 && value <= 0x0AFFFFFF) || (value >= 0xAC100000 && value <= 0xAC1FFFFF) || (value >= 0xC0A80000 && value <= 0xC0A8FFFF) {
		return true
	}
	return false
}

func GetLocalIP() string {
	hostname, err := os.Hostname()
	if nil != err {
		return "127.0.0.1"
	}
	ipp, err := net.LookupHost(hostname)
	if nil != err {
		return "127.0.0.1"
	}
	return ipp[0]
}
