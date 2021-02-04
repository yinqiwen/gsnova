module gsnova

go 1.15

require (
	github.com/NebulousLabs/go-upnp v0.0.0-20181203152547-b32978b8ccbf
	github.com/boltdb/bolt v1.3.1 // indirect
	github.com/dsnet/compress v0.0.1
	github.com/fsnotify/fsnotify v1.4.9
	github.com/golang/snappy v0.0.2
	github.com/google/btree v0.0.0-20180813153112-4030bb1f1f0c
	github.com/google/easypki v1.1.0
	github.com/gorilla/websocket v1.4.2
	github.com/juju/ratelimit v1.0.1
	github.com/klauspost/reedsolomon v1.9.11 // indirect
	github.com/lucas-clemente/quic-go v0.19.3
	github.com/miekg/dns v1.1.38
	github.com/templexxx/cpufeat v0.0.0-20180724012125-cef66df7f161 // indirect
	github.com/templexxx/xor v0.0.0-20191217153810-f85b25db303b // indirect
	github.com/tjfoc/gmsm v1.4.0 // indirect
	github.com/vmihailenco/msgpack v4.0.4+incompatible
	github.com/xtaci/kcp-go v5.4.20+incompatible
	github.com/xtaci/lossyconn v0.0.0-20200209145036-adba10fffc37 // indirect
	github.com/yinqiwen/fdns v0.0.0-20171017112320-12371043eaab
	github.com/yinqiwen/gotoolkit v0.0.0-20200524133648-3980351e079f
	github.com/yinqiwen/gsnova v0.0.0
	github.com/yinqiwen/pmux v0.0.0-20180811072835-da2f176b9f7a
	gitlab.com/NebulousLabs/fastrand v0.0.0-20181126182046-603482d69e40 // indirect
	gitlab.com/NebulousLabs/go-upnp v0.0.0-20181011194642-3a71999ed0d3 // indirect
	golang.org/x/crypto v0.0.0-20201221181555-eec23a3978ad
	golang.org/x/net v0.0.0-20210119194325-5f4716e94777
	golang.org/x/sys v0.0.0-20201119102817-f84b799fce68
	golang.org/x/time v0.0.0-20201208040808-7e3f01d25324
)

replace github.com/yinqiwen/gsnova v0.0.0 => ./
