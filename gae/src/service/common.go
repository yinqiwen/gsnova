package service

import (
	"bytes"
)

type EventSendService interface{
   GetMaxDataPackageSize()int
   Send(buf *bytes.Buffer);
}