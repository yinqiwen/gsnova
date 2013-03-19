package common

import (
	"fmt"
	"io"
	"log"
	"os"
	//"syscall"
)

var logWriter *MultiWriter

type MultiWriter struct {
	path    string
	file    *os.File
	writers []io.Writer
}

func (writer *MultiWriter) Write(p []byte) (n int, err error) {
	fmt.Print(string(p))
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
	}
	for _, writer := range writer.writers {
		writer.Write(p)
	}
	return len(p), nil
}

func initLogWriter(path string) *MultiWriter {
	writer := new(MultiWriter)
	writer.path = path
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	//fmt.Printf("file is %s\n", path)
	if err != nil {
		fmt.Println(err)
	} else {
		writer.file = file
		//Redirect crash stack dump to log file
		//syscall.Dup2(int(file.Fd()), 2)
	}
	logWriter = writer
	return writer
}


func AddLogWriter(writer io.Writer) {
	logWriter.writers = append(logWriter.writers, writer)
}

func InitLogger() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.SetOutput(initLogWriter(Home + Product + ".log"))
}
