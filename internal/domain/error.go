package domain

import "fmt"

var ErrReqNotSend = fmt.Errorf("Request not sent")
var ErrNotSuccessfulResp = fmt.Errorf("Request proceeded unsuccessfully")
var ErrNotFound = fmt.Errorf("Resource not found")
var ErrNotDirectory = fmt.Errorf("Resource is not directory")
var ErrCantHandle = fmt.Errorf("Handler can't handle")
