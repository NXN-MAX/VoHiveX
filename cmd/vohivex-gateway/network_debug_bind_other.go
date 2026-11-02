//go:build !linux

package main

import "fmt"

func cellularBindingAvailable() bool { return false }

func bindSocketToInterface(_ uintptr, _ string) error {
	return fmt.Errorf("network interface binding is only available on Linux")
}
