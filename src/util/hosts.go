package util

import ()

var mapping map[string]ListSelector = make(map[string]ListSelector)

func LoadHostMapping(file string) error{
   return nil
}

func GetHost(host string) string {
   h, exist := mapping[host]
   if exist{
      return h.Select().(string)
   }
   return host 
}
