//go:build !linux

package main

import (
	"context"
	"errors"
)

func probeSerialIMEI(context.Context, string) (string, error) {
	return "", errors.New("USB modem identity probing requires Linux")
}
