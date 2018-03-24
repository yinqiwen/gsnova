GSnova: Private Proxy Solution.    
[![Join the chat at https://gitter.im/gsnova/Lobby](https://badges.gitter.im/gsnova/Lobby.svg)](https://gitter.im/gsnova/Lobby?utm_source=badge&utm_medium=badge&utm_campaign=pr-badge&utm_content=badge)
[![Build Status](https://travis-ci.org/yinqiwen/gsnova.svg?branch=master)](https://travis-ci.org/yinqiwen/gsnova)

```                                                                                                                                              
                                                                    
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
                                                                    
                                                                                                                                   
```

# Features
- Multiple transport channel support
    - http/https
    - http2
    - websocket
    - tcp
    - tls
    - quic
    - kcp
    - ssh
- Multiplexing 
    - All proxy connections running over N persist proxy channel connections
- Simple PAC(Proxy Auto Config)
- Multiple Ciphers support
    - Chacha20Poly1305
    - Salsa20
    - AES128
- HTTP/Socks4/Socks5 Proxy
    - Local client running as HTTP/Socks4/Socks5 Proxy
- Transparent TCP/UDP Proxy
	- Transparent tcp/udp proxy implementation in pure golang
- Multi-hop Proxy


# Usage
**go1.9 is requied.**

## Compile
```shell
   go get -t -u -v github.com/yinqiwen/gsnova
```
There is also prebuilt binary release at [here](https://github.com/yinqiwen/gsnova/releases)

## Command Line  Usage
```
Usage of ./gsnova:
  -admin string
    	Admin listen address
  -client
    	Launch gsnova as client.
  -cmd
    	Launch gsnova  by command line without config file.
  -cnip string
    	China IP list. (default "./cnipset.txt")
  -conf string
    	Config file of gsnova.
  -hop value
    	Next proxy hop server to connect for client, eg:wss://xxx.paas.com
  -hosts string
    	Hosts file of gsnova client. (default "./hosts.json")
  -key string
    	Cipher key for transmission between local&remote. (default "809240d3a021449f6e67aa73221d42df942a308a")
  -listen value
    	Listen on address.
  -log string
    	Log file setting (default "color,gsnova.log")
  -pid string
    	PID file (default ".gsnova.pid")
  -ping_interval int
    	Channel ping interval seconds. (default 30)
  -server
    	Launch gsnova as server.
  -tls.cert string
    	TLS Cert file
  -tls.key string
    	TLS Key file
  -user string
    	Username for remote server to authorize. (default "gsnova")
  -version
    	Print version.
  -window string
    	Max mux stream window size, default 512K
  -window_refresh string
    	Mux stream window refresh size, default 32K
```

## Deploy & Run Server

```shell
   ./gsnova -cmd -server -listen tcp://:48100 -listen quic://:48100 -listen tls://:48101 -listen kcp://:48101 -listen http://:48102 -listen http2://:48103  -key 809240d3a021449f6e67aa73221d42df942a308a -user "*"
```
This would launch a running instance listening at serveral ports with different transport protocol.  

The server can also be deployed to serveral PAAS service like heroku/openshift and some docker host servce.  

## Deploy & Run Client(PC)

### Run From Command Line
```
   ./gsnova -cmd -client -listen :48100 -hop http2://app1.openshiftapps.com  -key 809240d3a021449f6e67aa73221d42df942a308a
```
This would launch a socks4/socks5/http proxy at port 48100 and use http2://app1.openshiftapps.com as next proxy hop.

### Run With Confguration

This is a sample for client.json, the `Key` and the `ServerList` need to be modified to match your server.
```json
{
	//this is just a example
	"Log": ["color","gsnova.log"],
	"UserAgent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_13_0) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/60.0.3112.101 Safari/537.36",
	//encrypt method can choose from none/auto/salsa20/chacha20poly1305/aes256-gcm
	//'auto' method would choose fastest encrypt method for current env
	"Cipher": {
		"Method": "auto",
		"Key": "809240d3a021449f6e67aa73221d42df942a308a"
	},
	//user name auth
	"User": "gsnova",
	"LocalDNS": {
		//only listen UDP
		"Listen": "127.0.0.1:5300",
		//for PAC rule 'IsCNIP', it would resolve the domain by 'TrustedDNS' if 'BlockedByGFW', and resolve the rest by 'FastDNS'
		"FastDNS": [
			"114.114.114.114"
		],
		"TrustedDNS": [
			"208.67.222.222:443",
			"208.67.220.220:443"
		]
	},
	"UDPGW": {
		//fake address, only used as udp protocol indicator
		"Addr": "20.20.20.20:1111"
	},
	"SNI": {
		//Used to redirect SNI host to another for sniffed SNI
		"Redirect": {
			//This fix "DF-DFERH-01" error in HW phone for google play 
			"services.googleapis.cn": "services.googleapis.com"
		}
	},
	//used to handle admin command from http client    
	"Admin": {
		//a local http server, do NOT expose this http server to public
		//listen on private IP instead of the default config 
		//eg: "Listen": "192.168.1.1:7788",
		"Listen": ":7788",
		//used to broadcast admin server address.
		"BroadcastAddr": "224.0.0.1:48100",
		"ConfigDir": "./android"
	},
	"GFWList": {
		"URL": "https://raw.githubusercontent.com/gfwlist/gfwlist/master/gfwlist.txt",
		"Proxy": "",
		"UserRule": []
	},
	"Proxy": [
		{
			"Local": ":48100",
			//Transparent proxy only works in linux system
			"Transparent": false,
			"PAC": [
				//// 'Direct/TLSDirect' MUST  proxy channel names confgiured below 
				//{"Protocol":["dns", "udp"],"Remote":"Direct"},
				// Support rules 'IsCNIP/InHosts/BlockedByGFW'
				//{"Rule":["InHosts"],"Remote":"TLSDirect"},
				//{"Rule":["!IsCNIP"],"Remote":"heroku"},
				//{"Rule":["BlockedByGFW"],"Remote":"heroku"},
				//{"Host":["*notexist_domain.com"],"Remote":"Reject"},
				//{"Host":["*"],"Remote":"Direct"},
				//{"URL":["*"],"Remote":"Direct"},
				//{"Method":["CONNECT"],"Remote":"Direct"}
				{
					"Remote": "default"
				}
			]
		}
	],
	"Channel": [
		{
			"Enable": true,
			"Name": "default",
			//Allowed server url with schema 'http/http2/https/ws/wss/tcp/tls/quic/kcp/ssh'
			//"ServerList":["quic://1.1.1.1:48101"],
			"ServerList": [
				"tcp://1.1.1.1:48100"
			],
			//if u are behind a HTTP proxy
			"Proxy": "",
			"ConnsPerServer": 3,
			//Unit: second
			"DialTimeout": 5,
			//Unit: second
			"ReadTimeout": 15,
			//Reconnect after 120s
			"ReconnectPeriod": 1800,
			//ReconnectPeriod rand adjustment, the real reconnect period is random value between [P - adjust, P + adjust] 
			"RCPRandomAdjustment": 10,
			//Send heartbeat msg to keep alive 
			"HeartBeatPeriod": 30,
			"Compressor": "none"
		}
	]
}
```
```
   ./gsnova -client -conf ./client.json
```

### Advanced Usage
#### Multi-Hop Proxy
GSnova support more than ONE remote server as the next hops, just add moren `-hop server` arguments to enable multi-hop proxy. 
```shell
   ./gsnova -cmd -client -listen :48101 -hop http2://app1.openshiftapps.com -hop wss://app2.herokuapp.com -key 809240d3a021449f6e67aa73221d42df942a308a
```
#### Transparent Proxy
- Edit iptables rules.
- It's only works on linux.


## Mobile Client(Android)
The client side can be compiled to android library by `gomobile`, eg:
```
   gomobile bind -target=android -a -v github.com/yinqiwen/gsnova/local/gsnova
```
Users can develop there own app by using the generated `gsnova.aar`.   
There is a very simple andorid app [gsnova-android-v0.27.3.1.zip](https://github.com/yinqiwen/gsnova/releases/download/v0.27.3/gsnova-android-v0.27.3.1.zip) which use `tun2socks` + `gsnova` to build. 




