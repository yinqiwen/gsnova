package upnp

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

type upnpNAT struct {
	serviceURL string
	ourIP      string
	root       *Root
}

type NAT interface {
	AddPortMapping(protocol string, externalPort, internalPort int, description string, timeout int) (err error)
	//AddPortMappingV2(protocol string, externalPort, internalPort int, description string, timeout int) (err error)
	DeletePortMapping(protocol string, externalPort int) (err error)
	GetExternalIPAddress() (string, error)
}

func Discover() (nat NAT, err error) {
	ssdp, err := net.ResolveUDPAddr("udp4", "239.255.255.250:1900")
	if err != nil {
		return
	}
	conn, err := net.ListenPacket("udp4", ":0")
	if err != nil {
		return
	}
	socket := conn.(*net.UDPConn)
	defer socket.Close()

	err = socket.SetDeadline(time.Now().Add(3 * time.Second))
	if err != nil {
		return
	}

	st := "ST: urn:schemas-upnp-org:device:InternetGatewayDevice:1\r\n"
	buf := bytes.NewBufferString(
		"M-SEARCH * HTTP/1.1\r\n" +
			"HOST: 239.255.255.250:1900\r\n" +
			st +
			"MAN: \"ssdp:discover\"\r\n" +
			"MX: 2\r\n\r\n")
	message := buf.Bytes()
	answerBytes := make([]byte, 1024)
	for i := 0; i < 3; i++ {
		_, err = socket.WriteToUDP(message, ssdp)
		if err != nil {
			return
		}
		var n int
		n, _, err = socket.ReadFromUDP(answerBytes)
		if err != nil {
			continue
			// socket.Close()
			// return
		}
		answer := string(answerBytes[0:n])

		if strings.Index(answer, "\r\n"+st) < 0 {
			continue
		}
		// HTTP header field names are case-insensitive.
		// http://www.w3.org/Protocols/rfc2616/rfc2616-sec4.html#sec4.2
		locString := "\r\nlocation: "
		answer = strings.ToLower(answer)
		locIndex := strings.Index(answer, locString)
		if locIndex < 0 {
			continue
		}
		loc := answer[locIndex+len(locString):]
		endIndex := strings.Index(loc, "\r\n")
		if endIndex < 0 {
			continue
		}
		locURL := loc[0:endIndex]

		var serviceURL string
		var roots *Root
		roots, serviceURL, err = getServiceURL(locURL)
		if err != nil {
			return
		}
		var ourIP string
		ourIP, err = getOurIP()
		if err != nil {
			return
		}
		nat = &upnpNAT{serviceURL: serviceURL, ourIP: ourIP, root: roots}
		return
	}
	err = errors.New("UPnP port discovery failed.")
	return
}

type Service struct {
	ServiceType string
	ControlURL  string
}

type DeviceList struct {
	Device []Device
}

type ServiceList struct {
	Service []Service
}

type Device struct {
	DeviceType  string
	DeviceList  DeviceList
	ServiceList ServiceList
}

//type SpecVersion struct {
//	Major  string
//	Minor  string
//}

type Root struct {
	//    SpecVersion SpecVersion 
	//    URLBase string
	Device Device
}

func getChildDevice(d *Device, deviceType string) *Device {
	dl := d.DeviceList.Device
	for i := 0; i < len(dl); i++ {
		if dl[i].DeviceType == deviceType {
			return &dl[i]
		}
	}
	return nil
}

func getChildService(d *Device, serviceType string) *Service {
	sl := d.ServiceList.Service
	for i := 0; i < len(sl); i++ {
		if sl[i].ServiceType == serviceType {
			return &sl[i]
		}
	}
	return nil
}

func getOurIP() (ip string, err error) {
	hostname, err := os.Hostname()
	//net.
	//	ifc, err := net.InterfaceByName("eth0")
	if err != nil {
		log.Printf("%v\n", err)
		return
	}
	//	addr, _ := ifc.Addrs()
	//	log.Printf("%s\n", addr[0].String())
	//	return "", errors.New("xxx")
	addrs, err := net.LookupHost(hostname)
	if err != nil {
		log.Printf("%v\n", err)
		return
	}

	return addrs[0], nil
}

func getServiceURL(rootURL string) (ro *Root, url string, err error) {
	r, err := http.Get(rootURL)
	if err != nil {
		return
	}
	defer r.Body.Close()
	if r.StatusCode >= 400 {
		err = errors.New(string(r.StatusCode))
		return
	}
	decoder := xml.NewDecoder(r.Body)
	var root Root
	//	cc := make([]byte, int(r.ContentLength))
	//	io.ReadFull(r.Body, cc)
	//    log.Printf("%s\n",string(cc))
	//xml.Header
	//   xml.NewDecoder(r.Body).
	currentDevice := &root.Device
	//currentDeviceList := currentDevice.DeviceList.Device
	//currentServiceList := currentDevice.ServiceList.Service
	var currentService *Service
	var currentElement *xml.StartElement
	top := true
	for {
		token, err := decoder.Token()
		if nil != err {
			break
		}
		//log.Printf("type :%T\n", token)

		start, ok := token.(xml.StartElement)
		if ok {
			currentElement = &start
			local := strings.ToLower(strings.TrimSpace(start.Name.Local))
			switch local {
			case "device":
				if !top {
					currentDevice.DeviceList.Device = append(currentDevice.DeviceList.Device, Device{})
					index := len(currentDevice.DeviceList.Device) - 1
					currentDevice = &currentDevice.DeviceList.Device[index]
				}
				top = false
			case "service":
				currentDevice.ServiceList.Service = append(currentDevice.ServiceList.Service, Service{})
				index := len(currentDevice.ServiceList.Service) - 1
				currentService = &currentDevice.ServiceList.Service[index]
			case "devicelist":
				currentDevice.DeviceList.Device = make([]Device, 0)
				//currentDeviceList = currentDevice.DeviceList.Device
			case "serviceList":
				currentDevice.ServiceList.Service = make([]Service, 0)
				//currentServiceList = currentDevice.ServiceList.Service
			}
		}

		content, ok := token.(xml.CharData)
		if ok && nil != currentElement {
			name := strings.ToLower(strings.TrimSpace(currentElement.Name.Local))
			v := strings.TrimSpace(string(content))
			if len(v) > 0 {
				//log.Printf("%s=%s\n", name, v)
				switch name {
				case "devicetype":
					currentDevice.DeviceType = v
				case "controlurl":
					currentService.ControlURL = v
				case "servicetype":
					currentService.ServiceType = v
				}
			}
		}
	}

	log.Printf("%v\n", root)
	a := &root.Device
	if a.DeviceType != "urn:schemas-upnp-org:device:InternetGatewayDevice:1" {
		err = errors.New("No InternetGatewayDevice")
		return
	}
	b := getChildDevice(a, "urn:schemas-upnp-org:device:WANDevice:1")
	if b == nil {
		err = errors.New("No WANDevice")
		return
	}
	c := getChildDevice(b, "urn:schemas-upnp-org:device:WANConnectionDevice:1")
	if c == nil {
		err = errors.New("No WANConnectionDevice")
		return
	}
	d := getChildService(c, "urn:schemas-upnp-org:service:WANIPConnection:1")
	if d == nil {
		err = errors.New("No WANIPConnection")
		return
	}
	url = combineURL(rootURL, d.ControlURL)
	ro = &root
	return
}

func combineURL(rootURL, subURL string) string {
	protocolEnd := "://"
	protoEndIndex := strings.Index(rootURL, protocolEnd)
	a := rootURL[protoEndIndex+len(protocolEnd):]
	rootIndex := strings.Index(a, "/")
	return rootURL[0:protoEndIndex+len(protocolEnd)+rootIndex] + subURL
}

func simpleUPnPcommand(url, service, action string, args map[string]string) (r *http.Response, err error) {
	soapAction := "\"" + service + "#" + action + "\""
	soapBody := "<?xml version=\"1.0\"?>\r\n" +
		"<SOAP-ENV:Envelope " +
		"xmlns:SOAP-ENV=\"http://schemas.xmlsoap.org/soap/envelope/\" " +
		"SOAP-ENV:encodingStyle=\"http://schemas.xmlsoap.org/soap/encoding/\">" +
		"<SOAP-ENV:Body>" +
		"<m:" + action + " xmlns:m=\"" + service + "\">"
	if len(args) > 0 {
		for k, v := range args {
			soapBody = fmt.Sprintf("%s<%s>%s</%s>", soapBody, k, v, k)
		}
	}
	soapBody = fmt.Sprintf("%s</m:%s>", soapBody, action)
	soapBody = soapBody + "</SOAP-ENV:Body></SOAP-ENV:Envelope>"
	req, err := http.NewRequest("POST", url, strings.NewReader(soapBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "text/xml ; charset=\"utf-8\"")
	req.Header.Set("User-Agent", "Darwin/10.0.0, UPnP/1.0, MiniUPnPc/1.3")
	//req.Header.Set("Transfer-Encoding", "chunked")
	req.Header.Set("SOAPAction", soapAction)
	req.Header.Set("Connection", "Close")
	req.Header.Set("Cache-Control", "no-cache")
	//req.Header.Set("Pragma", "no-cache")

	r, err = http.DefaultClient.Do(req)

	if r.StatusCode >= 400 {
		log.Printf("%v\n", r)
		err = errors.New("Error " + strconv.Itoa(r.StatusCode) + " for " + action)
		r = nil
		return
	}
	return
}

func soapRequest(url, function, message string) (r *http.Response, err error) {
	fullMessage := "<?xml version=\"1.0\" ?>" +
		"<s:Envelope xmlns:s=\"http://schemas.xmlsoap.org/soap/envelope/\" s:encodingStyle=\"http://schemas.xmlsoap.org/soap/encoding/\">\r\n" +
		"<s:Body>" + message + "</s:Body></s:Envelope>"

	req, err := http.NewRequest("POST", url, strings.NewReader(fullMessage))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "text/xml ; charset=\"utf-8\"")
	req.Header.Set("User-Agent", "Darwin/10.0.0, UPnP/1.0, MiniUPnPc/1.3")
	//req.Header.Set("Transfer-Encoding", "chunked")
	req.Header.Set("SOAPAction", "\"urn:schemas-upnp-org:service:WANIPConnection:1#"+function+"\"")
	req.Header.Set("Connection", "Close")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Pragma", "no-cache")

	// log.Stderr("soapRequest ", req)

	r, err = http.DefaultClient.Do(req)
	if r.Body != nil {
		defer r.Body.Close()
	}

	if r.StatusCode >= 400 {
		// log.Stderr(function, r.StatusCode)
		err = errors.New("Error " + strconv.Itoa(r.StatusCode) + " for " + function)
		r = nil
		return
	}
	return
}

func (n *upnpNAT) GetStatusInfo() (err error) {

	message := "<u:GetStatusInfo xmlns:u=\"urn:schemas-upnp-org:service:WANIPConnection:1\">\r\n" +
		"</u:GetStatusInfo>"

	var response *http.Response
	response, err = soapRequest(n.serviceURL, "GetStatusInfo", message)
	if err != nil {
		return
	}

	// TODO: Write a soap reply parser. It has to eat the Body and envelope tags...

	response.Body.Close()
	return
}

func (n *upnpNAT) GetExternalIPAddress() (string, error) {
	args := make(map[string]string)
	response, err := simpleUPnPcommand(n.serviceURL, "urn:schemas-upnp-org:service:WANIPConnection:1", "GetExternalIPAddress", args)
	if err != nil {
		return "", err
	}
	decoder := xml.NewDecoder(response.Body)
	retriveIP := false
	ip := ""
	for {
		t, err := decoder.Token()
		if nil != err {
			break
		}
		start, ok := t.(xml.StartElement)
		if ok && strings.EqualFold(start.Name.Local, "NewExternalIPAddress") {
			retriveIP = true
		}
		content, ok := t.(xml.CharData)
		if ok && retriveIP{
		   ip = strings.TrimSpace(string(content))
		   break
		}
	}
	return ip, nil
}

func (n *upnpNAT) AddPortMapping(protocol string, externalPort, internalPort int, description string, timeout int) (err error) {
	// A single concatenation would brake ARM compilation.
	args := make(map[string]string)
	args["NewRemoteHost"] = "" // wildcard, any remote host matches
	args["NewExternalPort"] = strconv.Itoa(externalPort)
	args["NewProtocol"] = strings.ToUpper(protocol)
	args["NewInternalPort"] = strconv.Itoa(internalPort)
	args["NewInternalClient"] = n.ourIP
	args["NewEnabled"] = strconv.Itoa(1)
	args["NewPortMappingDescription"] = description
	args["NewLeaseDuration"] = "0"
	//log.Printf(n.root.Device.ServiceList.Service[0].ServiceType)

	var response *http.Response
	response, err = simpleUPnPcommand(n.serviceURL, "urn:schemas-upnp-org:service:WANIPConnection:1", "AddPortMapping", args)
	if err != nil {
		return
	}
	//	cc := make([]byte, int(response.ContentLength))
	//    response.Body.Read(cc)

	// TODO: check response to see if the port was forwarded
	// log.Println(message, response)
	_ = response
	response.Body.Close()
	return
}

func (n *upnpNAT) DeletePortMapping(protocol string, externalPort int) (err error) {
	args := make(map[string]string)
	args["NewRemoteHost"] = "" // wildcard, any remote host matches
	args["NewExternalPort"] = strconv.Itoa(externalPort)
	args["NewProtocol"] = strings.ToUpper(protocol)

	var response *http.Response
	response, err = simpleUPnPcommand(n.serviceURL, "urn:schemas-upnp-org:service:WANIPConnection:1", "DeletePortMapping", args)
	if err != nil {
		return
	}

	//	var response *http.Response
	//	response, err = soapRequest(n.serviceURL, "DeletePortMapping", message)
	//	if err != nil {
	//		return
	//	}

	// TODO: check response to see if the port was deleted
	// log.Println(message, response)
	_ = response
	response.Body.Close()
	return
}

//func (n *upnpNAT) AddPortMapping(protocol string, externalPort, internalPort int, description string, timeout int) (err error) {
//	// A single concatenation would brake ARM compilation.
//	message := "<u:AddPortMapping xmlns:u=\"urn:schemas-upnp-org:service:WANIPConnection:1\">\r\n" +
//		"<NewRemoteHost></NewRemoteHost><NewExternalPort>" + strconv.Itoa(externalPort)
//	message += "</NewExternalPort><NewProtocol>" + protocol + "</NewProtocol>"
//	message += "<NewInternalPort>" + strconv.Itoa(internalPort) + "</NewInternalPort>" +
//		"<NewInternalClient>" + n.ourIP + "</NewInternalClient>" +
//		"<NewEnabled>1</NewEnabled><NewPortMappingDescription>"
//	message += description +
//		"</NewPortMappingDescription><NewLeaseDuration>" + strconv.Itoa(timeout) +
//		"</NewLeaseDuration></u:AddPortMapping>"
//
//	var response *http.Response
//	response, err = soapRequest(n.serviceURL, "AddPortMapping", message)
//	if err != nil {
//		return
//	}
//
//	// TODO: check response to see if the port was forwarded
//	// log.Println(message, response)
//	_ = response
//	return
//}

//func (n *upnpNAT) DeletePortMapping(protocol string, externalPort int) (err error) {
//
//	message := "<u:DeletePortMapping xmlns:u=\"urn:schemas-upnp-org:service:WANIPConnection:1\">\r\n" +
//		"<NewRemoteHost></NewRemoteHost><NewExternalPort>" + strconv.Itoa(externalPort) +
//		"</NewExternalPort><NewProtocol>" + protocol + "</NewProtocol>" +
//		"</u:DeletePortMapping>"
//
//	var response *http.Response
//	response, err = soapRequest(n.serviceURL, "DeletePortMapping", message)
//	if err != nil {
//		return
//	}
//
//	// TODO: check response to see if the port was deleted
//	// log.Println(message, response)
//	_ = response
//	return
//}
