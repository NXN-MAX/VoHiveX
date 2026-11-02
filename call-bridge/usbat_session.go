package main

import "time"

// usbATCommandFunc executes one AT command while the underlying USB handle is
// held exclusively. Callers can validate one response before deciding whether
// it is safe to issue the next command, without another goroutine interleaving
// a command or replacing the handle between the validation and the write.
type usbATCommandFunc func(command string, timeout time.Duration) (string, error)

type usbATPhysicalIdentity struct {
	VendorID  int
	ProductID int
	Location  uint32
}
