#!/bin/sh
insmod nf_tproxy_core
insmod xt_TPROXY

iptables -t nat -N GSNOVA
iptables -t nat -A GSNOVA -p tcp -m mark --mark 48100 -j RETURN
iptables -t nat -A GSNOVA -d 0.0.0.0/8 -j RETURN
iptables -t nat -A GSNOVA -d 10.0.0.0/8 -j RETURN
iptables -t nat -A GSNOVA -d 127.0.0.0/8 -j RETURN
iptables -t nat -A GSNOVA -d 169.254.0.0/16 -j RETURN
iptables -t nat -A GSNOVA -d 172.16.0.0/12 -j RETURN
iptables -t nat -A GSNOVA -d 192.168.0.0/16 -j RETURN
iptables -t nat -A GSNOVA -d 224.0.0.0/4 -j RETURN
iptables -t nat -A GSNOVA -d 240.0.0.0/4 -j RETURN
iptables -t nat -A GSNOVA -p tcp -m set --match-set cnipset dst -j RETURN
#iptables -t nat -A GSNOVA -p tcp -j REDIRECT --to-ports 48100
iptables -t nat -A GSNOVA -p tcp -j REDIRECT -s 192.168.1.5 --to-ports 48100
iptables -t nat -I PREROUTING -p tcp  -j GSNOVA


# for openwrt
#iptables -t nat -I PREROUTING -p tcp  -j GSNOVA
# for local linux  
#iptables -t nat -A OUTPUT -p tcp -j GSNOVA
iptables -t mangle -N GSNOVA
iptables -t mangle -A GSNOVA -p udp -m mark --mark 48100 -j RETURN
iptables -t mangle -A GSNOVA -p udp --dport 53 -s 192.168.1.5 -j TPROXY --on-port 48101 --tproxy-mark 0x01/0x01
#iptables -t mangle -A GSNOVA -p udp --dport 53 -s 192.168.1.75 -j TPROXY --on-port 48101 --tproxy-mark 0x01/0x01
iptables -t mangle -A PREROUTING -j GSNOVA