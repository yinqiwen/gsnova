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


# Usage

## Deploy Server

```shell
   go get -t -u -v github.com/yinqiwen/gsnova/remote/server
   go build github.com/yinqiwen/gsnova/remote/server
   ./server -tcp :48100 -quic :48100 -tls :48101 -kcp :48102 -http :48102 -http2 :48102  -key 809240d3a021449f6e67aa73221d42df942a308a -allow "*"
```
This would launch a running instance listening at serveral ports with different transport protocol.  

The server can also be deployed to serveral PAAS service like heroku/openshift and some docker host servce.  

## Deploy Client(PC)
```shell
   go get -t -u -v github.com/yinqiwen/gsnova/local/client
   mkdir gsnova_client; cd gsnova_client
   go build github.com/yinqiwen/gsnova/local/client
   cp $GOPATH/github.com/yinqiwen/gsnova/*.json ./
   #...edit client.json...
   ./client
```
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
		"Listen": "127.0.0.1:48100",
		//for PAC rule 'IsCNIP', it would resolve the domain by 'TrustedDNS' if 'BlockedByGFW', and resolve the rest by 'FastDNS'
		"FastDNS": [
			"114.114.114.114"
		],
		"TrustedDNS": [
			"208.67.222.222:443",
			"208.67.220.220:443"
		],
		"CacheSize": 1024,
		"TCPConnect": false
	},
	//used to replace forward dns query's target DNS server addr 
	"RemoteDNS": {
		"TrustedDNS": [
			"8.8.8.8",
			"8.8.4.4"
		]
	},
	"UDPGW": {
		//fake address, only used as udp protocol indicator
		"Addr": "20.20.20.20:1111",
		//since gsnova sniff SNI for https, 'Host' for http, return fake record for dns query would make it run faster for http/https traffic
		"LocalDNSRecord": {
			"*": "111.111.111.111"
		}
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


## Mobile Client(Android)
The client side can be compiled to android library by `gomobile`, eg:
```
   gomobile bind -target=android -a -v github.com/yinqiwen/gsnova/local/gsnova
```
Users can develop there own app by using the generated `gsnova.aar`.   
There is a very simple andorid app [gsnova-android-v0.27.3.1.zip](https://github.com/yinqiwen/gsnova/releases/download/v0.27.3/gsnova-android-v0.27.3.1.zip) which use `tun2socks` + `gsnova` to build. 




