//go:build darwin && cgo

// Package mavousb uses MaVo's MIT-licensed IOKit AT transport without drivers,
// parent-device resets, USB composition changes, or cellular network setup.
package mavousb

/*
#cgo LDFLAGS: -framework IOKit -framework CoreFoundation
#include <stdlib.h>
#include "CModemBridge.h"
*/
import "C"

import (
	"errors"
	"strings"
	"sync"
	"time"
	"unsafe"
)

type Client struct {
	mu    sync.Mutex
	modem *C.MaVoModem
}

func Open() (*Client, error) {
	m := C.mavo_modem_create()
	if m == nil {
		return nil, errors.New("USB allocation failed")
	}
	if C.mavo_modem_open(m) != 0 {
		err := errors.New(C.GoString(C.mavo_modem_last_error(m)))
		C.mavo_modem_destroy(m)
		return nil, err
	}
	return &Client{modem: m}, nil
}
func (c *Client) ExecuteATSilent(command string, timeout time.Duration) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.modem == nil {
		return "", errors.New("USB AT transport closed")
	}
	if strings.ContainsAny(command, "\r\n\x00") {
		return "", errors.New("invalid AT command")
	}
	cmd := C.CString(command)
	defer C.free(unsafe.Pointer(cmd))
	out := make([]byte, 65536)
	rc := C.mavo_modem_command(c.modem, cmd, C.int(timeout.Milliseconds()), (*C.char)(unsafe.Pointer(&out[0])), C.size_t(len(out)))
	if rc != 0 {
		return "", errors.New(C.GoString(C.mavo_modem_last_error(c.modem)))
	}
	return C.GoString((*C.char)(unsafe.Pointer(&out[0]))), nil
}
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.modem != nil {
		C.mavo_modem_destroy(c.modem)
		c.modem = nil
	}
	return nil
}
