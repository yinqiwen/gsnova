#!/bin/sh
iptables -t nat -N GSNOVA
#iptables -t nat -A GSNOVA -p tcp --dport 48100 -j RETURN

iptables -t nat -A GSNOVA -d 10.14.87.100 -j RETURN

iptables -t nat -A GSNOVA -d 0.0.0.0/8 -j RETURN
iptables -t nat -A GSNOVA -d 10.0.0.0/8 -j RETURN
iptables -t nat -A GSNOVA -d 127.0.0.0/8 -j RETURN
iptables -t nat -A GSNOVA -d 169.254.0.0/16 -j RETURN
iptables -t nat -A GSNOVA -d 172.16.0.0/12 -j RETURN
iptables -t nat -A GSNOVA -d 192.168.0.0/16 -j RETURN
iptables -t nat -A GSNOVA -d 224.0.0.0/4 -j RETURN
iptables -t nat -A GSNOVA -d 240.0.0.0/4 -j RETURN
iptables -t nat -A GSNOVA -p tcp -j REDIRECT --to-ports 48100

# for openwrt
#iptables -t nat -I PREROUTING -p tcp  -j GSNOVA
# for local linux  
iptables -t nat -A OUTPUT -p tcp -j GSNOVA