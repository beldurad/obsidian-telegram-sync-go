package domain

import "fmt"

var ErrClient = fmt.Errorf("Client error")
var ErrNotFound = fmt.Errorf("Resource not found")
var ErrNotDirectory = fmt.Errorf("Resource is not directory")
var ErrUnknown = fmt.Errorf("Unknown error")
var ErrDb error = fmt.Errorf("DB error")
var ErrBadArgument error = fmt.Errorf("Bad argument error")
