package logger

import (
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/fatih/color"
	//"syscall"
)

type colorConsoleWriter struct {
	color *color.Color
	w     *os.File
}

func (writer *colorConsoleWriter) Write(p []byte) (n int, err error) {
	if nil != writer.color {
		writer.color.Fprintf(writer.w, "%s", string(p))
	} else {
		return writer.w.Write(p)
	}
	return len(p), nil
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
var colorConsoleLogger [5]*log.Logger

func Debug(format string, v ...interface{}) {
	if withFile {
		log.Output(2, fmt.Sprintf(format, v...))
	}
	if withColorConsole {
		colorConsoleLogger[0].Output(2, fmt.Sprintf(format, v...))
	}

}
func Notice(format string, v ...interface{}) {
	if withFile {
		log.Output(2, fmt.Sprintf(format, v...))
	}
	if withColorConsole {
		colorConsoleLogger[1].Output(2, fmt.Sprintf(format, v...))
	}
}

func Info(format string, v ...interface{}) {
	if withFile {
		log.Output(2, fmt.Sprintf(format, v...))
	}

	if withColorConsole {
		colorConsoleLogger[2].Output(2, fmt.Sprintf(format, v...))
	}
}

func Error(format string, v ...interface{}) {
	if withFile {
		log.Output(2, fmt.Sprintf(format, v...))
	}
	if withColorConsole {
		colorConsoleLogger[3].Output(2, fmt.Sprintf(format, v...))
	}
}

func Fatal(format string, v ...interface{}) {
	if withFile {
		log.Output(2, fmt.Sprintf(format, v...))
	}
	if withColorConsole {
		colorConsoleLogger[4].Output(2, fmt.Sprintf(format, v...))
	}
	os.Exit(1)
}

func init() {
	logFlag := log.LstdFlags | log.Lshortfile
	log.SetFlags(logFlag)
	colorConsoleLogger[0] = log.New(&colorConsoleWriter{color: nil, w: os.Stdout}, "", logFlag)
	colorConsoleLogger[1] = log.New(&colorConsoleWriter{color: color.New(color.Reset, color.FgYellow), w: os.Stdout}, "", logFlag)
	colorConsoleLogger[2] = log.New(&colorConsoleWriter{color: color.New(color.Reset, color.FgGreen), w: os.Stdout}, "", logFlag)
	colorConsoleLogger[3] = log.New(&colorConsoleWriter{color: color.New(color.Bold, color.FgRed), w: os.Stdout}, "", logFlag)
	colorConsoleLogger[4] = log.New(&colorConsoleWriter{color: color.New(color.Bold, color.FgRed), w: os.Stdout}, "", logFlag)
}
