package logger

import (
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	//"syscall"
)

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
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	ws := make([]io.Writer, 0)
	for _, name := range output {
		if strings.EqualFold(name, "stdout") {
			ws = append(ws, os.Stdout)
		} else if strings.EqualFold(name, "stderr") {
			ws = append(ws, os.Stderr)
		} else {
			ws = append(ws, initLogWriter(name))
		}
	}
	if len(ws) > 0 {
		logWriter = io.MultiWriter(ws...)
		log.SetOutput(logWriter)
	}
}

var logWriter io.Writer

func GetLoggerWriter() io.Writer {
	return logWriter
}
