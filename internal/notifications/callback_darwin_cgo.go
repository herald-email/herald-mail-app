//go:build darwin && cgo

package notifications

/*
#include <stdlib.h>
*/
import "C"

import "sync"

var darwinResponseMu sync.Mutex
var darwinResponseCh chan string

func setDarwinResponseChannel(ch chan string) {
	darwinResponseMu.Lock()
	defer darwinResponseMu.Unlock()
	darwinResponseCh = ch
}

//export heraldNotificationActivated
func heraldNotificationActivated(link *C.char) {
	deepLink := C.GoString(link)
	darwinResponseMu.Lock()
	ch := darwinResponseCh
	darwinResponseMu.Unlock()
	if ch == nil {
		return
	}
	select {
	case ch <- deepLink:
	default:
	}
}
