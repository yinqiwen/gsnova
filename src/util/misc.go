package util

import (
	"crypto/dsa"
	"math/big"
	"misc/myasn1"
	"regexp"
	"strconv"
	"strings"
)

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
