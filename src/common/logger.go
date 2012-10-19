package common

import (
	"fmt"
	"log"
	"os"
)

type FileConsoleWriter struct {
	path string
	file *os.File
}

func (writer *FileConsoleWriter) Write(p []byte) (n int, err error) {
	fmt.Print(string(p))
	if nil != writer.file {
		writer.file.Write(p)
		fi, err := writer.file.Stat()
		//5MB
		if nil == err && fi.Size() >= 1*1024*1024 {
			os.Remove(writer.path + ".1")
			writer.file.Close()
			os.Rename(writer.path, writer.path+".1")
			writer.file, _ = os.OpenFile(writer.path, os.O_CREATE|os.O_APPEND, 0755)
		}
	}
	return len(p), nil
}

func initLogWriter(path string) *FileConsoleWriter {
	writer := new(FileConsoleWriter)
	writer.path = path
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND, 0755)
	//fmt.Printf("file is %s\n", path)
	if err != nil {
		fmt.Println(err)
	} else {
		writer.file = file
	}
	return writer
}

func InitLogger() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.SetOutput(initLogWriter(Home + Product + ".log"))
}
