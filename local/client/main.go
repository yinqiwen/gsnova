package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/yinqiwen/gsnova/common/logger"
	"github.com/yinqiwen/gsnova/local/gsnova"
)

func printASCIILogo() {

	logo := `
	        ___          ___          ___          ___         ___          ___     
	       /\  \        /\  \        /\__\        /\  \       /\__\        /\  \    
	      /::\  \      /::\  \      /::|  |      /::\  \     /:/  /       /::\  \   
	     /:/\:\  \    /:/\ \  \    /:|:|  |     /:/\:\  \   /:/  /       /:/\:\  \  
	    /:/  \:\  \  _\:\~\ \  \  /:/|:|  |__  /:/  \:\  \ /:/__/  ___  /::\~\:\  \ 
	   /:/__/_\:\__\/\ \:\ \ \__\/:/ |:| /\__\/:/__/ \:\__\|:|  | /\__\/:/\:\ \:\__\
	   \:\  /\ \/__/\:\ \:\ \/__/\/__|:|/:/  /\:\  \ /:/  /|:|  |/:/  /\/__\:\/:/  /
	    \:\ \:\__\   \:\ \:\__\      |:/:/  /  \:\  /:/  / |:|__/:/  /      \::/  / 
	     \:\/:/  /    \:\/:/  /      |::/  /    \:\/:/  /   \::::/__/       /:/  /  
	      \::/  /      \::/  /       /:/  /      \::/  /     ~~~~          /:/  /   
	       \/__/        \/__/        \/__/        \/__/                    \/__/    
	`
	fmt.Println(logo)

	// fmt.Println(" .----------------.  .----------------.  .-----------------. .----------------.  .----------------.  .----------------. ")
	// fmt.Println("| .--------------. || .--------------. || .--------------. || .--------------. || .--------------. || .--------------. |")
	// fmt.Println("| |    ______    | || |    _______   | || | ____  _____  | || |     ____     | || | ____   ____  | || |      __      | |")
	// fmt.Println("| |  .' ___  |   | || |   /  ___  |  | || ||_   \\|_   _| | || |   .'    `.   | || ||_  _| |_  _| | || |     /  \\     | |")
	// fmt.Println("| | / .'   \\_|   | || |  |  (__ \\_|  | || |  |   \\ | |   | || |  /  .--.  \\  | || |  \\ \\   / /   | || |    / /\\ \\    | |")
	// fmt.Println("| | | |    ____  | || |   '.___`-.   | || |  | |\\ \\| |   | || |  | |    | |  | || |   \\ \\ / /    | || |   / ____ \\   | |")
	// fmt.Println("| | \\ `.___]  _| | || |  |`\\____) |  | || | _| |_\\   |_  | || |  \\  `--'  /  | || |    \\ ' /     | || | _/ /    \\ \\_ | |")
	// fmt.Println("| |  `._____.'   | || |  |_______.'  | || ||_____|\\____| | || |   `.____.'   | || |     \\_/      | || ||____|  |____|| |")
	// fmt.Println("| |              | || |              | || |              | || |              | || |              | || |              | |")
	// fmt.Println("| '--------------' || '--------------' || '--------------' || '--------------' || '--------------' || '--------------' |")
	// fmt.Println(" '----------------'  '----------------'  '----------------'  '----------------'  '----------------'  '----------------' ")
}

func main() {
	path, err := filepath.Abs(os.Args[0])
	if nil != err {
		fmt.Println(err)
		return
	}
	home, _ := filepath.Split(path)
	conf := flag.String("conf", "./client.json", "Config file of gsnova client.")
	hosts := flag.String("hosts", "./hosts.json", "Hosts file of gsnova client.")
	pid := flag.String("pid", ".gsnova.pid", "PID file")
	flag.Parse()

	printASCIILogo()

	err = gsnova.StartLocalProxy(home, *conf, *hosts, true)
	if nil != err {
		logger.Error("Start gsnova error:%v", err)
	} else {
		if len(*pid) > 0 {
			ioutil.WriteFile(*pid, []byte(fmt.Sprintf("%d", os.Getpid())), os.ModePerm)
		}
		ch := make(chan int)
		<-ch
	}
}
