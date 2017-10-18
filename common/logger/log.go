package logger

import (
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	//"github.com/fatih/color"
	//"syscall"
)

type colorConsoleWriter struct {
	prefix  string
	postfix string
	w       *os.File
}

func (writer *colorConsoleWriter) Write(p []byte) (n int, err error) {
	if len(writer.prefix) > 0 {
		fmt.Fprint(writer.w, writer.prefix)
		n, err = writer.w.Write(p)
		fmt.Fprint(writer.w, writer.postfix)
		return
	}
	return writer.w.Write(p)
}

type logFileWriter struct {
	path string
	file *os.File
}

func (writer *logFileWriter) close() {
	if nil != writer.file {
		writer.file.Close()
	}
}

func (writer *logFileWriter) Write(p []byte) (n int, err error) {
	if nil != writer.file {
		_, err := writer.file.Write(p)
		if nil != err {
			fmt.Printf("Failed to write logfile for reason:%v\n", err)
		}
		fi, err := writer.file.Stat()
		//5MB
		if nil == err && fi.Size() >= 1*1024*1024 {
			os.Remove(writer.path + ".1")
			writer.file.Close()
			os.Rename(writer.path, writer.path+".1")
			writer.file, _ = os.OpenFile(writer.path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
		}
	} else {
		fmt.Printf("No log file inited for %s \n", writer.path)
	}
	return len(p), nil
}

func initLogWriter(path string) *logFileWriter {
	writer := new(logFileWriter)
	writer.path = path
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	//fmt.Printf("file is %s\n", path)
	if err != nil {
		fmt.Println(err)
	} else {
		writer.file = file
	}
	return writer
}

func IsDebugEnable() bool {
	return true
}

func InitLogger(output []string) {
	withFile = false
	ws := make([]io.Writer, 0)
	for _, name := range output {
		if strings.EqualFold(name, "stdout") {
			ws = append(ws, os.Stdout)
			withFile = true
		} else if strings.EqualFold(name, "console") {
			ws = append(ws, os.Stdout)
			withFile = true
		} else if strings.EqualFold(name, "color") {
			//ws = append(ws, os.Stderr)
			withColorConsole = true
		} else {
			ws = append(ws, initLogWriter(name))
			withFile = true
		}
	}
	if len(ws) > 0 {
		log.SetOutput(io.MultiWriter(ws...))
	}
}

var withColorConsole bool
var withFile bool
var colorConsoleLogger *log.Logger

func Debug(format string, v ...interface{}) {
	if withFile {
		log.Output(2, fmt.Sprintf(format, v...))
	}
	if withColorConsole {
		colorConsoleLogger.Output(2, fmt.Sprintf(format, v...))
	}

}
func Notice(format string, v ...interface{}) {
	if withFile {
		log.Output(2, fmt.Sprintf(format, v...))
	}
	if withColorConsole {
		setNoticeColor()
		colorConsoleLogger.Output(2, fmt.Sprintf(format, v...))
		unsetNoticeColor()
	}
}

func Info(format string, v ...interface{}) {
	if withFile {
		log.Output(2, fmt.Sprintf(format, v...))
	}

	if withColorConsole {
		setINFOColor()
		colorConsoleLogger.Output(2, fmt.Sprintf(format, v...))
		unsetINFOColor()
	}
}

func Error(format string, v ...interface{}) {
	if withFile {
		log.Output(2, fmt.Sprintf(format, v...))
	}
	if withColorConsole {
		setErrorColor()
		colorConsoleLogger.Output(2, fmt.Sprintf(format, v...))
		unsetErrorColor()
	}
}

func Fatal(format string, v ...interface{}) {
	if withFile {
		log.Output(2, fmt.Sprintf(format, v...))
	}
	if withColorConsole {
		setErrorColor()
		colorConsoleLogger.Output(2, fmt.Sprintf(format, v...))
		unsetErrorColor()
	}
	os.Exit(1)
}

func init() {
	logFlag := log.LstdFlags | log.Lshortfile
	log.SetFlags(logFlag)
	log.SetOutput(os.Stdout)
	withFile = true
	colorConsoleLogger = log.New(&colorConsoleWriter{w: os.Stdout}, "", logFlag)
}
