package service

import (
	"appengine"
	"appengine/datastore"
	//"appengine/memcache"
	"appengine/capability"
	"event"
	//"bytes"
	"strings"
	//"codec"
	//"strconv"
)

var UserTable map[string]*event.User = make(map[string]*event.User)
var GroupTable map[string]*event.Group = make(map[string]*event.Group)

func User2PropertyList(user *event.User) datastore.PropertyList {
	var ret = make(datastore.PropertyList, 0, 6)
	ret = append(ret, datastore.Property{
		Name:  "Name",
		Value: user.Email,
	})
	ret = append(ret, datastore.Property{
		Name:  "Passwd",
		Value: user.Passwd,
	})
	ret = append(ret, datastore.Property{
		Name:  "Group",
		Value: user.Group,
	})
	ret = append(ret, datastore.Property{
		Name:  "AuthToken",
		Value: user.AuthToken,
	})
	var tmp string = ""
	for key, _ := range user.BlackList {
		tmp += key
		tmp += ";"
	}
	ret = append(ret, datastore.Property{
		Name:  "BlackList",
		Value: tmp,
	})
	return ret
}

func PropertyList2User(props datastore.PropertyList) *event.User {
	user := new(event.User)
	user.BlackList = make(map[string]string)
	for _, v := range props {
		switch v.Name {
		case "Name":
			user.Email = v.Value.(string)
		case "Passwd":
			user.Passwd = v.Value.(string)
		case "Group":
			user.Group = v.Value.(string)
		case "AuthToken":
			user.AuthToken = v.Value.(string)
		case "BlackList":
			str := v.Value.(string)
			ss := strings.Split(str, ";")
			for _, s := range ss {
				s = strings.TrimSpace(s)
				if len(s) > 0 {
					user.BlackList[s] = s
				}
			}
		}
	}
	return user
}

func Group2PropertyList(group *event.Group) datastore.PropertyList {
	var ret = make(datastore.PropertyList, 0, 2)
	ret = append(ret, datastore.Property{
		Name:  "Name",
		Value: group.Name,
	})
	var tmp string = ""
	for key, _ := range group.BlackList {
		tmp += key
		tmp += ";"
	}
	ret = append(ret, datastore.Property{
		Name:  "BlackList",
		Value: tmp,
	})
	return ret
}

func PropertyList2Group(props datastore.PropertyList) *event.Group {
	group := new(event.Group)
	group.BlackList = make(map[string]string)
	for _, v := range props {
		switch v.Name {
		case "Name":
			group.Name = v.Value.(string)
		case "BlackList":
			str := v.Value.(string)
			ss := strings.Split(str, ";")
			for _, s := range ss {
				s = strings.TrimSpace(s)
				if len(s) > 0 {
					group.BlackList[s] = s
				}
			}
		}
	}
	return group
}

const USER_CACHE_KEY_PREFIX = "User:"
const GROUP_CACHE_KEY_PREFIX = "Group:"

func SaveUser(ctx appengine.Context, user *event.User) {
	props := User2PropertyList(user)
	key := datastore.NewKey(ctx, "ProxyUser", user.Email, 0, nil)
	_, err := datastore.Put(ctx, key, &props)
	if err != nil {
		ctx.Errorf("Failed to put user:%s data in datastore:%s", user.Email, err.Error())
		return
	}
	//var buf bytes.Buffer
	//user.Encode(&buf)
	//memitem1 := &memcache.Item{
	//	Key:   USER_CACHE_KEY_PREFIX + user.Email,
	//	Value: buf.Bytes(),
	//}
	//memitem2 := &memcache.Item{
	//	Key:   USER_CACHE_KEY_PREFIX + user.AuthToken,
	//	Value: buf.Bytes(),
	//}
	// Add the item to the memcache, if the key does not already exist
	//memcache.Set(ctx, memitem1)
	//memcache.Set(ctx, memitem2)
	UserTable[USER_CACHE_KEY_PREFIX+user.AuthToken] = user
	UserTable[USER_CACHE_KEY_PREFIX+user.Email] = user
}

func GetUserFromCache(ctx appengine.Context, name string) *event.User {
	user, exist := UserTable[USER_CACHE_KEY_PREFIX+name]
	if exist {
		return user
	}
	//if item, err := memcache.Get(ctx, USER_CACHE_KEY_PREFIX+name); err == nil {
	//	buf := bytes.NewBuffer(item.Value)
	//	user = new(event.User)
	//	if user.Decode(buf) {
	//		UserTable[USER_CACHE_KEY_PREFIX+name] = user
	//		return user
	//	}
	//}
	return nil
}

func GetUserWithName(ctx appengine.Context, name string) *event.User {
	user := GetUserFromCache(ctx, name)
	if nil != user {
		return user
	}
	var item datastore.PropertyList
	key := datastore.NewKey(ctx, "ProxyUser", name, 0, nil)
	if err := datastore.Get(ctx, key, &item); err != nil {
		ctx.Errorf("Failed to get user:%s data in datastore:%s", name, err.Error())
		return nil
	}
	user = PropertyList2User(item)
	//var buf bytes.Buffer
	//user.Encode(&buf)
	//memitem := &memcache.Item{
	//	Key:   USER_CACHE_KEY_PREFIX + user.Email,
	//	Value: buf.Bytes(),
	//}
	//memitem2 := &memcache.Item{
	//	Key:   USER_CACHE_KEY_PREFIX + user.AuthToken,
	//	Value: buf.Bytes(),
	//}
	//memcache.Set(ctx, memitem)
	//memcache.Set(ctx, memitem2)
	return user
}

func GetUserWithToken(ctx appengine.Context, token string) *event.User {
	user := GetUserFromCache(ctx, token)
	if nil != user {
		return user
	}
	q := datastore.NewQuery("ProxyUser").Filter("AuthToken =", token)
	for t := q.Run(ctx); ; {
		var x datastore.PropertyList
		_, err := t.Next(&x)
		if err == datastore.Done {
			break
		}
		if err != nil {
			ctx.Errorf("Failed to get user:%s data in datastore:%s", token, err.Error())
			return nil
		}
		user = PropertyList2User(x)
		return user
	}
	return nil
}

func GetAllUsers(ctx appengine.Context) []*event.User {
	q := datastore.NewQuery("ProxyUser")
	users := make([]*event.User, 0, 10)
	for t := q.Run(ctx); ; {
		var x datastore.PropertyList
		_, err := t.Next(&x)
		if err == datastore.Done {
			break
		}
		if err != nil {
			ctx.Errorf("Failed to get all user data in datastore:%s", err.Error())
			break
		}
		user := PropertyList2User(x)
		users = append(users, user)
	}
	return users
}

func SaveGroup(ctx appengine.Context, grp *event.Group) {
	props := Group2PropertyList(grp)
	key := datastore.NewKey(ctx, "ProxyGroup", grp.Name, 0, nil)
	_, err := datastore.Put(ctx, key, &props)
	if err != nil {
		ctx.Errorf("Failed to put group:%s data in datastore:%s", grp.Name, err.Error())
		return
	}
	//var buf bytes.Buffer
	//grp.Encode(&buf)
	//memitem1 := &memcache.Item{
	//	Key:   GROUP_CACHE_KEY_PREFIX + grp.Name,
	//	Value: buf.Bytes(),
	//}
	// Add the item to the memcache, if the key does not already exist
	//memcache.Set(ctx, memitem1)
	GroupTable[GROUP_CACHE_KEY_PREFIX+grp.Name] = grp
}

func GetGroup(ctx appengine.Context, name string) *event.Group {
	group, exist := GroupTable[GROUP_CACHE_KEY_PREFIX+name]
	if exist {
		return group
	}
	//if item, err := memcache.Get(ctx, GROUP_CACHE_KEY_PREFIX+name); err == nil {
	//	buf := bytes.NewBuffer(item.Value)
	//	group = new(event.Group)
	//	if group.Decode(buf) {
	//		GroupTable[GROUP_CACHE_KEY_PREFIX+name] = group
	//		return group
	//	}
	//}
	var item datastore.PropertyList
	key := datastore.NewKey(ctx, "ProxyGroup", name, 0, nil)
	if err := datastore.Get(ctx, key, &item); err != nil {
		ctx.Errorf("Failed to get group:%s data in datastore:%s", name, err.Error())
		return nil
	}
	group = PropertyList2Group(item)
	//var buf bytes.Buffer
	//group.Encode(&buf)
	//memitem := &memcache.Item{
	//	Key:   GROUP_CACHE_KEY_PREFIX + group.Name,
	//	Value: buf.Bytes(),
	//}
	//memcache.Set(ctx, memitem)
	return group
}

func GetAllGroups(ctx appengine.Context) []*event.Group {
	q := datastore.NewQuery("ProxyGroup")
	groups := make([]*event.Group, 0, 10)
	for t := q.Run(ctx); ; {
		var x datastore.PropertyList
		_, err := t.Next(&x)
		if err == datastore.Done {
			break
		}
		if err != nil {
			ctx.Errorf("Failed to get all user data in datastore:%s", err.Error())
			break
		}
		group := PropertyList2Group(x)
		groups = append(groups, group)
	}
	return groups
}

func DeleteUser(ctx appengine.Context, user *event.User) {
	key := datastore.NewKey(ctx, "ProxyUser", user.Email, 0, nil)
	datastore.Delete(ctx, key)
	key1 := USER_CACHE_KEY_PREFIX + user.Email
	key2 := USER_CACHE_KEY_PREFIX + user.AuthToken
	delete(UserTable, key1)
	delete(UserTable, key2)
	//memcache.Delete(ctx, key1)
	//memcache.Delete(ctx, key2)
}

func DeleteGroup(ctx appengine.Context, group *event.Group) {
	key := datastore.NewKey(ctx, "ProxyGroup", group.Name, 0, nil)
	datastore.Delete(ctx, key)
	key1 := GROUP_CACHE_KEY_PREFIX + group.Name
	delete(GroupTable, key1)
	//memcache.Delete(ctx, key1)
}

func IsRootUser(ctx appengine.Context, token string) bool {
	user := GetUserWithToken(ctx, token)
	if nil != user && user.Email == "Root" {
		return true
	}
	return false
}

func UserAuthServiceAvailable(ctx appengine.Context) bool {
	if !capability.Enabled(ctx, "datastore_v3", "*") {
		return false
	}
	if !capability.Enabled(ctx, "memcache", "*") {
		return false
	}
	return true
}
