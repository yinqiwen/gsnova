package util

import (
	"strconv"
	"strings"
)

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
