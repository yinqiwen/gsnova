package util

import (
	"bytes"
	"crypto/dsa"
	"fmt"
	"math/big"
	"misc/myasn1"
	"net"
	"net/http"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

var freeList = make(chan *bytes.Buffer, 20)
var serverChan = make(chan *bytes.Buffer)

func GetBuffer() *bytes.Buffer {
	var b *bytes.Buffer
	// Grab a buffer if available; allocate if not.
	select {
	case b = <-freeList:
		b.Reset()
	default:
		// None free, so allocate a new one.
		b = new(bytes.Buffer)
	}
	return b
}

func RecycleBuffer(b *bytes.Buffer) {
	select {
	case freeList <- b:
		// Buffer on free list; nothing more to do.
	default:
		// Free list full, just carry on.
	}
}

func IsTimeoutError(err error) bool {
	if neterr, ok := err.(net.Error); ok && neterr.Timeout() {
		return true
	}
	return false
}

type dsaPrivateKey struct {
	Version       int
	P, Q, G, Y, X *big.Int
}

func DecodeDSAPrivateKEy(der []byte) (key *dsa.PrivateKey, err error) {
	var priv dsaPrivateKey
	rest, err := myasn1.Unmarshal(der, &priv)
	if len(rest) > 0 {
		err = myasn1.SyntaxError{Msg: "trailing data"}
		return
	}
	if err != nil {
		return
	}
	key = new(dsa.PrivateKey)
	key.P = priv.P
	key.Q = priv.Q
	key.G = priv.G
	key.Y = priv.Y
	key.X = priv.X
	return
}

func RegexpReplace(str, replace string, regex *regexp.Regexp, count int) string {
	if 0 == count {
		return str
	}
	if regex != nil {
		if count < 0 {
			return regex.ReplaceAllString(str, replace)
		}
		return regex.ReplaceAllStringFunc(str, func(s string) string {
			if count != 0 {
				count -= 1
				return replace
			}
			return s
		})
	}
	return str
}

func RegexpPatternReplace(str, pattern, replace string, count int) string {
	if 0 == count {
		return str
	}
	if tmp, err := regexp.Compile(pattern); err == nil {
		if count < 0 {
			return tmp.ReplaceAllString(str, replace)
		}
		return tmp.ReplaceAllStringFunc(str, func(s string) string {
			if count != 0 {
				count -= 1
				return replace
			}
			return s
		})
	}
	return str
}

func ParseRangeHeaderValue(value string) (startPos, endPos int) {
	vs := strings.Split(value, "=")
	vs = strings.Split(vs[1], "-")
	startPos, _ = strconv.Atoi(vs[0])
	endPos, _ = strconv.Atoi(vs[1])
	return
}

func PrepareRegexp(rule string, only_star bool) (*regexp.Regexp, error) {
	rule = strings.TrimSpace(rule)
	rule = strings.Replace(rule, ".", "\\.", -1)
	if !only_star {
		rule = strings.Replace(rule, "?", "\\?", -1)
	}
	rule = strings.Replace(rule, "*", ".*", -1)
	return regexp.Compile(rule)
}

func GetURLString(req *http.Request, with_method bool) string {
	if nil == req {
		return ""
	}
	str := req.URL.String()
	if len(req.URL.Scheme) == 0 && strings.EqualFold(req.Method, "Connect") && len(req.URL.Path) == 0 {
		str = fmt.Sprintf("https://%s", req.Host)
	}
	if !strings.HasPrefix(str, "http://") && !strings.HasPrefix(str, "https://") {
		scheme := req.URL.Scheme
		if len(req.URL.Scheme) == 0 {
			scheme = "http"

		}
		str = fmt.Sprintf("%s://%s%s", scheme, req.Host, str)
	}
	if with_method {
		return fmt.Sprintf("%s %s", req.Method, str)
	}
	return str
}

func ParseContentRangeHeaderValue(value string) (startPos, endPos, length int) {
	rangeVal := strings.Split(value, " ")[1]
	vs := strings.Split(rangeVal, "/")
	length, _ = strconv.Atoi(vs[1])
	vs = strings.Split(vs[0], "-")
	startPos, _ = strconv.Atoi(vs[0])
	if len(vs) == 1 {
		endPos = -1
	} else {
		endPos, _ = strconv.Atoi(vs[1])
	}
	return
}

func WildcardMatch(text string, pattern string) bool {
	cards := strings.Split(pattern, "*")
	for _, str := range cards {
		idx := strings.Index(text, str)
		if idx == -1 {
			return false
		}
		text = strings.TrimLeft(text, str+"*")
	}
	return true
}

func OpenBrowser(urlstr string) error {
	if runtime.GOOS == "windows" {
		return exec.Command("cmd.exe", "/C", "start "+urlstr).Run()
	}

	if runtime.GOOS == "darwin" {
		return exec.Command("open", urlstr).Run()
	}

	return exec.Command("xdg-open", urlstr).Run()
}
