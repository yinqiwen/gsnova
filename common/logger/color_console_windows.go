// +build windows

package logger

import (
	"os"
	"strings"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

const (
	foregroundBlue      = uint16(0x0001)
	foregroundGreen     = uint16(0x0002)
	foregroundRed       = uint16(0x0004)
	foregroundYellow    = uint16(0x0006)
	foregroundIntensity = uint16(0x0008)
	backgroundBlue      = uint16(0x0010)
	backgroundGreen     = uint16(0x0020)
	backgroundRed       = uint16(0x0040)
	backgroundIntensity = uint16(0x0080)
	underscore          = uint16(0x8000)
	reset               = uint16(0x07)

	foregroundMask = foregroundBlue | foregroundGreen | foregroundRed | foregroundIntensity
	backgroundMask = backgroundBlue | backgroundGreen | backgroundRed | backgroundIntensity
)

const (
	fileNameInfo uintptr = 2
	fileTypePipe         = 3
)

var (
	kernel32                         = syscall.NewLazyDLL("kernel32.dll")
	procSetConsoleTextAttribute      = kernel32.NewProc("SetConsoleTextAttribute")
	procGetConsoleScreenBufferInfo   = kernel32.NewProc("GetConsoleScreenBufferInfo")
	procGetFileInformationByHandleEx = kernel32.NewProc("GetFileInformationByHandleEx")
	procGetFileType                  = kernel32.NewProc("GetFileType")
	stdoutFD                         = os.Stdout.Fd()
)

// Check pipe name is used for cygwin/msys2 pty.
// Cygwin/MSYS2 PTY has a name like:
//   \{cygwin,msys}-XXXXXXXXXXXXXXXX-ptyN-{from,to}-master
func isCygwinPipeName(name string) bool {
	token := strings.Split(name, "-")
	if len(token) < 5 {
		return false
	}

	if token[0] != `\msys` && token[0] != `\cygwin` {
		return false
	}

	if token[1] == "" {
		return false
	}

	if !strings.HasPrefix(token[2], "pty") {
		return false
	}

	if token[3] != `from` && token[3] != `to` {
		return false
	}

	if token[4] != "master" {
		return false
	}

	return true
}

// IsCygwinTerminal() return true if the file descriptor is a cygwin or msys2
// terminal.
func IsCygwinTerminal(fd uintptr) bool {
	if procGetFileInformationByHandleEx == nil {
		return false
	}

	// Cygwin/msys's pty is a pipe.
	ft, _, e := syscall.Syscall(procGetFileType.Addr(), 1, fd, 0, 0)
	if ft != fileTypePipe || e != 0 {
		return false
	}

	var buf [2 + syscall.MAX_PATH]uint16
	r, _, e := syscall.Syscall6(procGetFileInformationByHandleEx.Addr(),
		4, fd, fileNameInfo, uintptr(unsafe.Pointer(&buf)),
		uintptr(len(buf)*2), 0, 0)
	if r == 0 || e != 0 {
		return false
	}

	l := *(*uint32)(unsafe.Pointer(&buf))
	return isCygwinPipeName(string(utf16.Decode(buf[2 : 2+l/2])))
}

func setConsoleTextAttribute(hConsoleOutput uintptr, wAttributes uint16) bool {
	ret, _, _ := procSetConsoleTextAttribute.Call(
		hConsoleOutput,
		uintptr(wAttributes))
	return ret != 0
}

func setINFOColor() {
	if IsCygwinTerminal(stdoutFD) {
		setBoldGreen()
	} else {
		setConsoleTextAttribute(stdoutFD, foregroundGreen|foregroundIntensity)
	}
}
func unsetINFOColor() {
	if IsCygwinTerminal(stdoutFD) {
		unsetBoldGreen()
	} else {
		setConsoleTextAttribute(stdoutFD, reset)
	}
}
func setNoticeColor() {
	if IsCygwinTerminal(stdoutFD) {
		setYellow()
	} else {
		setConsoleTextAttribute(stdoutFD, foregroundYellow)
	}
}
func unsetNoticeColor() {
	if IsCygwinTerminal(stdoutFD) {
		unsetYellow()
	} else {
		setConsoleTextAttribute(stdoutFD, reset)
	}
}
func setErrorColor() {
	if IsCygwinTerminal(stdoutFD) {
		setBoldRed()
	} else {
		setConsoleTextAttribute(stdoutFD, foregroundRed|foregroundIntensity)
	}
}
func unsetErrorColor() {
	if IsCygwinTerminal(stdoutFD) {
		unsetBoldRed()
	} else {
		setConsoleTextAttribute(stdoutFD, reset)
	}
}
