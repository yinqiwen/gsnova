package logger

import (
	"fmt"
	"os"
)

func setBoldGreen() {
	fmt.Fprint(os.Stdout, "\x1b[1m\x1b[32m")
}

func unsetBoldGreen() {
	fmt.Fprint(os.Stdout, "\x1b[21m\x1b[0m")
}

func setBoldRed() {
	fmt.Fprint(os.Stdout, "\x1b[1m\x1b[31m")
}

func unsetBoldRed() {
	fmt.Fprint(os.Stdout, "\x1b[21m\x1b[0m")
}

func setYellow() {
	fmt.Fprint(os.Stdout, "\x1b[33m")
}

func unsetYellow() {
	fmt.Fprint(os.Stdout, "\x1b[0m")
}
