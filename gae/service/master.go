package service

import (
	"appengine"
	"appengine/datastore"
	"appengine/mail"
	"appengine/urlfetch"
	"bytes"
	"event"
	"io"
	"math/rand"
	"misc"
	"strings"
)

func isValidSnovaSite(c appengine.Context, appid string) bool {
	client := urlfetch.Client(c)
	resp, err := client.Get("http://" + appid + ".appspot.com")
	if err == nil {
		if resp.StatusCode == 200 {
			var tmp bytes.Buffer
			if _, err = io.Copy(&tmp, resp.Body); nil == err {
				if !strings.Contains(tmp.String(), "snova") {
					c.Errorf("Definitly invalid appid:%s", appid)
					return false
				} else {
					return true
				}
			}
		} else if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			c.Errorf("Definitly invalid:%s for response:%v", appid, resp)
			return false
		}
		c.Errorf("Failed to verify appid:%s for response:%v", appid, resp)
		return true
	}
	c.Errorf("Failed to verify appid:%s for reason:%v", appid, err)
	return true
}

type AppIDItem struct {
	AppID string
	Email string
}

func AppIDItem2PropertyList(item *AppIDItem) datastore.PropertyList {
	var ret = make(datastore.PropertyList, 0, 2)
	ret = append(ret, datastore.Property{
		Name:  "AppID",
		Value: item.AppID,
	})
	ret = append(ret, datastore.Property{
		Name:  "Email",
		Value: item.Email,
	})
	return ret
}

func PropertyList2AppIDItem(props datastore.PropertyList) *AppIDItem {
	item := new(AppIDItem)
	for _, v := range props {
		switch v.Name {
		case "AppID":
			item.AppID = v.Value.(string)
		case "Email":
			item.Email = v.Value.(string)
		}
	}
	return item
}

var sharedAppIdItems []*AppIDItem

func sendMail(ctx appengine.Context, addr string, subject string, content string) {
	appid := appengine.AppID(ctx)
	sendcontent := "Hi,\r\n\r\n"
	sendcontent += content
	sendcontent += "Thanks again. admin@" + appid + ".appspot.com"
	msg := &mail.Message{
		Sender:  "admin@" + appid + ".appspotmail.com",
		To:      []string{addr},
		Cc:      []string{"yinqiwen@gmail.com"},
		Subject: subject,
		Body:    sendcontent,
	}
	if err := mail.Send(ctx, msg); err != nil {
		ctx.Errorf("Couldn't send email: %v", err)
	}
}

func getSharedAppItem(ctx appengine.Context, appid string) *AppIDItem {
	var slen int = len(sharedAppIdItems)
	for i := 0; i < slen; i++ {
		item := sharedAppIdItems[i]
		if item.AppID == appid {
			return item
		}
	}
	return nil
}

func saveSharedAppItem(ctx appengine.Context, item *AppIDItem) {
	//sharedAppIdItems.Push(item)
	sharedAppIdItems = append(sharedAppIdItems, item)
	key := datastore.NewKey(ctx, "SharedAppID", item.AppID, 0, nil)
	props := AppIDItem2PropertyList(item)
	_, err := datastore.Put(ctx, key, &props)
	if err != nil {
		ctx.Errorf("Failed to share appid:%s in datastore:%s", item.AppID)
	}
}

func deleteSharedAppItem(ctx appengine.Context, item *AppIDItem) {
	var slen int = len(sharedAppIdItems)
	for i := 0; i < slen; i++ {
		tmp := sharedAppIdItems[i]
		if tmp.AppID == item.AppID {
			//sharedAppIdItems.Delete(i)
			sharedAppIdItems = append(sharedAppIdItems[:i], sharedAppIdItems[i+1:]...)
			break
		}
	}
	key := datastore.NewKey(ctx, "SharedAppID", item.AppID, 0, nil)
	err := datastore.Delete(ctx, key)
	if err != nil {
		ctx.Errorf("Failed to delete appid:%s for reason:%v", item.AppID, err)
	}
}

func shareAppID(ctx appengine.Context, appid, email string) event.Event {
	resev := new(event.AdminResponseEvent)
	if !isValidSnovaSite(ctx, appid) {
		resev.ErrorCause = "This AppId is not a valid snova appid."
		return resev
	}
	item := getSharedAppItem(ctx, appid)
	if nil != item {
		resev.ErrorCause = "This AppId is already shared!"
		return resev
	}
	item = new(AppIDItem)
	item.AppID = appid
	item.Email = email
	saveSharedAppItem(ctx, item)
	sendMail(ctx, email, "Thanks for sharing AppID:"+appid+"!",
		"Thank you for sharing your appid!")
	resev.Response = "Success"
	return resev
}

func unShareAppID(ctx appengine.Context, appid, email string) event.Event {
	resev := new(event.AdminResponseEvent)
	item := getSharedAppItem(ctx, appid)
	if nil == item {
		resev.ErrorCause = "This appid is not shared before!"
		return resev
	}

	if item.Email != email && item.Email != misc.MasterAdminEmail {
		resev.ErrorCause = "The input email address is not equal the share email address."
		return resev
	}
	item = new(AppIDItem)
	item.AppID = appid
	item.Email = email
	deleteSharedAppItem(ctx, item)
	email_content := "Your appid:" + appid + " is unshared from snova master."
	if item.Email == misc.MasterAdminEmail {
		email_content = email_content + "\nWe noticied that your shared appid:" + appid + " is not a valid Snova Server AppID."
	}
	sendMail(ctx, email, "Unshared AppID:"+appid+"!", email_content)
	resev.Response = "Success"
	return resev
}

func InitMasterService(ctx appengine.Context) {
	if len(sharedAppIdItems) == 0 {
		q := datastore.NewQuery("SharedAppID")
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
			item := PropertyList2AppIDItem(x)
			//sharedAppIdItems.Push(item)
			sharedAppIdItems = append(sharedAppIdItems, item)
		}
	}
}

func HandleShareEvent(ctx appengine.Context, ev *event.ShareAppIDEvent) event.Event {
	if ev.Operation == event.APPID_SHARE {
		return shareAppID(ctx, ev.AppId, ev.Email)
	}
	return unShareAppID(ctx, ev.AppId, ev.Email)
}

func RetrieveAppIds(ctx appengine.Context) event.Event {
	ctx.Infof("Shared items length  :%d", len(sharedAppIdItems))
	for len(sharedAppIdItems) > 0 {
		res := new(event.RequestAppIDResponseEvent)
		res.AppIDs = make([]string, 1)
		index := rand.Intn(len(sharedAppIdItems))
		item := sharedAppIdItems[index]
		if !isValidSnovaSite(ctx, item.AppID) {
			unShareAppID(ctx, item.AppID, item.Email)
			continue
		}
		res.AppIDs[0] = item.AppID
		return res
	}
	resev := new(event.AdminResponseEvent)
	resev.ErrorCause = "No shared appid."
	return resev
}

func RetrieveAllAppIds(ctx appengine.Context) event.Event {
	if len(sharedAppIdItems) > 0 {
		res := new(event.RequestAppIDResponseEvent)
		res.AppIDs = make([]string, len(sharedAppIdItems))
		for i := 0; i < len(sharedAppIdItems); i++ {
			res.AppIDs[i] = sharedAppIdItems[i].AppID
		}
		return res
	}
	resev := new(event.AdminResponseEvent)
	resev.ErrorCause = "No shared appid."
	return resev
}
