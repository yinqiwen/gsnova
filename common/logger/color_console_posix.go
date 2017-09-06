// +build !windows

package logger

func setINFOColor() {
	setBoldGreen()
}
func unsetINFOColor() {
	unsetBoldGreen()
}
func setNoticeColor() {
	setYellow()
}
func unsetNoticeColor() {
	unsetYellow()
}
func setErrorColor() {
	setBoldRed()
}
func unsetErrorColor() {
	unsetBoldRed()
}
