//go:build linux

package main

import "golang.org/x/sys/unix"

func cellularBindingAvailable() bool { return true }

func bindSocketToInterface(fd uintptr, interfaceName string) error {
	return unix.SetsockoptString(int(fd), unix.SOL_SOCKET, unix.SO_BINDTODEVICE, interfaceName)
}
