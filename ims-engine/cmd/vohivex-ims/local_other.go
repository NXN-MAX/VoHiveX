//go:build !darwin || !cgo

package main

import "errors"

func newLocalCore(device string) (*coreClient, error) {
	return nil, errors.New("local USB requires a macOS cgo build")
}
