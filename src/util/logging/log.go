package logging

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
)

const MaxLogFileSize int64 = 1024 * 1024

var DebugEnable bool = true
var snovaLogger *log.Logger
var snovaLogFilePath string

func InitLogger(path string) {
	loghome, _ := filepath.Split(path)
	os.Mkdir(loghome, 0755)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND, 0755)
	if err != nil {
		fmt.Println(err)
	} else {
		snovaLogger = log.New(file, "", log.LstdFlags)
		snovaLogFilePath = path
	}
}

func rollLogFile() {
	if nil != snovaLogger {
		fi, err := os.Stat(snovaLogFilePath)
		if nil == err {
			if fi.Size() >= MaxLogFileSize {
				err = os.Rename(snovaLogFilePath, snovaLogFilePath+".1")
				if nil == err {
					InitLogger(snovaLogFilePath)
				}
			}
		}
	}
}

func Info(format string, v ...interface{}) {
	str := fmt.Sprintf("[INFO]"+format, v...)
	log.Println(str)
	if nil != snovaLogger {
		snovaLogger.Println(str)
		rollLogFile()
	}
}

func Debug(format string, v ...interface{}) {
	if !DebugEnable {
		return
	}
	str := fmt.Sprintf("[DEBUG]"+format, v...)
	log.Println(str)
	if nil != snovaLogger {
		snovaLogger.Printf(str)
		rollLogFile()
	}
}

func Error(format string, v ...interface{}) {
	str := fmt.Sprintf("[ERROR]"+format, v...)
	log.Println(str)
	if nil != snovaLogger {
		snovaLogger.Printf(str)
		rollLogFile()
	}
}
