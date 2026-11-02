//go:build darwin && cgo

package main

import (
	"github.com/NXN-MAX/VoHiveX/ims-engine/internal/mavousb"
	"github.com/iniwex5/vowifi-go/runtimehost/simauth"
	"github.com/iniwex5/vowifi-go/runtimehost/simtransport"
)

func newLocalCore(device string) (*coreClient, error) {
	usb, err := mavousb.Open()
	if err != nil {
		return nil, err
	}
	c := &coreClient{device: device, local: usb}
	c.adapter = simtransport.NewAdapter(c)
	c.aka = simauth.NewAKAProvider(c.adapter)
	return c, nil
}
