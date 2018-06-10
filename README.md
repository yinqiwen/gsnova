GSnova: Private Proxy Solution & Network Troubleshooting Tool.    
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
    - tcp/tls
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
- TLS man-in-the-middle(MITM) Proxy
- HTTP(S) Packet Capture for Web Debugging
	- Log HTTP(S) Packets in file
	- Forward HTTP(S) Packets to Remote HTTP Server
- P2P/P2S2P Proxy
    - P2P: Use TCP NAT tunnel for direct P2P commnunication if possible
    - P2S2P: Use middle server for two peers to communication
    - Use UPNP to expose port for remote p2p peer if possible.
- Low-memory Environments Support
    - Use less than 20MB RSS memory at client/server side


# Usage
**go1.9 or higher is requied.**

## Compile
```shell
   go get -t -u -v github.com/yinqiwen/gsnova
```
There is also prebuilt binary release at [here](https://github.com/yinqiwen/gsnova/releases)

## Command Line  Usage
```
Usage of ./gsnova:
  -admin string
    	Client Admin listen address
  -blackList value
    	Proxy blacklist item config
  -client
    	Launch gsnova as client.
  -cmd
    	Launch gsnova by command line without config file.
  -cnip string
    	China IP list. (default "./cnipset.txt")
  -conf string
    	Config file of gsnova.
  -forward value
    	Forward connection to specified address
  -hosts string
    	Hosts file of gsnova client. (default "./hosts.json")
  -httpdump.dst string
    	HTTP Dump destination file or http url
  -httpdump.filter value
    	HTTP Dump Domain Filter, eg:*.google.com
  -key string
    	Cipher key for transmission between local&remote. (default "809240d3a021449f6e67aa73221d42df942a308a")
  -listen value
    	Listen on address.
  -log string
    	Log file setting (default "color,gsnova.log")
  -mitm
    	Launch gsnova as a MITM Proxy
  -ots string
    	Online trouble shooting listen address
  -p2p string
    	P2P Token.
  -pid string
    	PID file (default ".gsnova.pid")
  -ping_interval int
    	Channel ping interval seconds. (default 30)
  -pprof string
    	PProf trouble shooting listen address
  -proxy string
    	Proxy setting to connect remote server.
  -remote value
    	Next remote proxy hop server to connect for client, eg:wss://xxx.paas.com
  -servable
    	Client as a proxy server for peer p2p client
  -server
    	Launch gsnova as server.
  -stream_idle int
    	Mux stream idle timout seconds. (default 10)
  -tls.cert string
    	TLS Cert file
  -tls.key string
    	TLS Key file
  -upnp int
    	UPNP port to expose for p2p.
  -user string
    	Username for remote server to authorize. (default "gsnova")
  -version
    	Print version.
  -whitelist value
    	Proxy whitelist item config
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

The server can also be deployed to serveral PAAS service like heroku/openshift and some docker host service.  

## Deploy & Run Client

### Run From Command Line
```
   ./gsnova -cmd -client -listen :48100 -remote http2://app1.openshiftapps.com  -key 809240d3a021449f6e67aa73221d42df942a308a
```
This would launch a socks4/socks5/http proxy at port 48100 and use http2://app1.openshiftapps.com as next proxy hop.

### Run With Confguration

This is a sample for [client.json](https://github.com/yinqiwen/gsnova/blob/master/client.json), the `Key` and the `ServerList` need to be modified to match your server.
```
   ./gsnova -client -conf ./client.json
```

### Advanced Usage
#### Multi-Hop Proxy
GSnova support more than ONE remote server as the next hops, just add more `-remote server` arguments to enable multi-hop proxy.     
This would use `http2://app1.openshiftapps.com` as the first proxy ho and use `wss://app2.herokuapp.com` as the final proxy hop.
```shell
   ./gsnova -cmd -client -listen :48101 -remote http2://app1.openshiftapps.com -remote wss://app2.herokuapp.com -key 809240d3a021449f6e67aa73221d42df942a308a
```
#### Transparent Proxy
- Edit iptables rules.
- It's only works on linux.

#### MITM Proxy
GSnova support running the client as a MITM proxy to capture HTTP(S) packets for web debuging.    
This would capture HTTP(S) traffic packets into local dist file `httpdump.log`.
```shell
   ./gsnova -cmd -client -listen :48101 -remote direct -mitm -httpdump.dst ./httpdump.log -httpdump.filter "*.google.com" -httpdump.filter "*.facebook.com"
```

#### P2P/P2S2P Proxy
P2P/P2S2P Proxy can help you to connect two nodes, and use one of them as a tcp proxy server for the other one.  This feature can be used for scenarios like:       
- Expose any tcp based service behind a NAT or firewall to a specific node in the internet.

There are 3 nodes which should install/run gsnova, a middle server(S) with public IP address, two client nodes(A & B)  behind a NAT or firewall.  
For the middle server(S), run as a server with a cipher key.
```shell
   ./gsnova -cmd -server  -listen tcp://:48103 -key p2pkey -log color
```
For the node(B) as a proxy server, run as a client to connect server with a P2P token:
```shell
  ./gsnova -cmd -client -servable -key p2pkey -remote tcp://<server ip>:48103 -p2p testp2p  -log color  
```
For the node(A) as a client for peer proxy server, run as a client to connect server with same P2P token:
```shell
  ./gsnova -cmd -client -listen :7788 -key p2pkey -remote tcp://<server ip>:48103 -p2p testp2p -log color  
```
If there is no error, now the node A with listen address :7788 can be used as a http/socks4/socks5 proxy to access servers behind a NAT or firewall which node B located in.       

And in gsnova, it would try to run with P2P mode first, if it's not pissible, it would use P2S2P mode which would use the middle server to forward tcp stream to remote peeer.  

## Mobile Client(Android)
The client side can be compiled to android library by `gomobile`, eg:
```
   gomobile bind -target=android -a -v github.com/yinqiwen/gsnova/local/gsnova
```
Users can develop there own app by using the generated `gsnova.aar`.   
There is a very simple andorid app [gsnova-android-v0.27.3.1.zip](https://github.com/yinqiwen/gsnova/releases/download/v0.27.3/gsnova-android-v0.27.3.1.zip) which use `tun2socks` + `gsnova` to build. 




