#!/bin/sh
wget -c http://ftp.apnic.net/stats/apnic/delegated-apnic-latest
cat delegated-apnic-latest | awk -F '|' '/CN/&&/ipv4/ {print $4 "/" 32-log($5)/log(2)}' > cnipset.txt

ipset -N cnipset hash:net maxelem 65536
for ip in $(cat 'cnipset.txt'); do
  ipset add cnipset $ip
done