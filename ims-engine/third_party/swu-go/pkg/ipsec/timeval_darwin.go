package ipsec

import "syscall"

func durationToTimeval(sec, usec int64) syscall.Timeval {
	return syscall.Timeval{Sec: sec, Usec: int32(usec)}
}
