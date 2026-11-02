package main

import (
	"errors"
	"fmt"
)

const (
	usbClassAudio                = 0x01
	usbAudioStreamingSubclass    = 0x02
	usbClassVendorSpecific       = 0xff
	usbADBSubclass               = 0x42
	usbADBProtocol               = 0x01
	usbEndpointDirectionIn       = 0x80
	usbEndpointNumberMask        = 0x0f
	usbEndpointTransferTypeMask  = 0x03
	usbEndpointTransferTypeBulk  = 0x02
	usbADBConservativeAltSetting = 0
)

var (
	errUSBADBInterfaceNotFound  = errors.New("standard USB ADB interface not found")
	errUSBADBInterfaceAmbiguous = errors.New("multiple standard USB ADB interfaces found")
)

// usbADBEndpointDescriptor contains only the endpoint fields needed to decide
// whether an alternate setting is a safe ADB transport. Keeping this type free
// of libusb makes the hardware selector independently testable.
type usbADBEndpointDescriptor struct {
	Address    byte
	Attributes byte
}

// usbADBAlternateDescriptor is one interface alternate setting from the active
// USB configuration. Device identity and physical location have already been
// checked by the caller.
type usbADBAlternateDescriptor struct {
	InterfaceNumber  byte
	AlternateSetting byte
	Class            byte
	Subclass         byte
	Protocol         byte
	Endpoints        []usbADBEndpointDescriptor
}

type usbADBCandidate struct {
	InterfaceNumber  int
	AlternateSetting int
	EndpointIn       byte
	EndpointOut      byte
}

// selectQDC507ADBInterface accepts only the standard Android ADB descriptor
// triple and exactly one bulk endpoint in each direction in the same alt.
//
// It intentionally accepts alt 0 only. This avoids changing an interface's
// active alternate setting and, in particular, prevents an Audio Streaming
// alt from being repurposed as an ADB transport. Interface numbers are not
// stable identities and therefore are not part of the predicate.
func selectQDC507ADBInterface(alternates []usbADBAlternateDescriptor) (usbADBCandidate, error) {
	var candidates []usbADBCandidate
	for _, alternate := range alternates {
		if alternate.AlternateSetting != usbADBConservativeAltSetting {
			continue
		}
		// Reject USB Audio Streaming explicitly, even though the exact ADB
		// descriptor check below would also exclude it.
		if alternate.Class == usbClassAudio && alternate.Subclass == usbAudioStreamingSubclass {
			continue
		}
		if alternate.Class != usbClassVendorSpecific ||
			alternate.Subclass != usbADBSubclass ||
			alternate.Protocol != usbADBProtocol {
			continue
		}

		var endpointIn, endpointOut byte
		var bulkInCount, bulkOutCount int
		for _, endpoint := range alternate.Endpoints {
			if endpoint.Attributes&usbEndpointTransferTypeMask != usbEndpointTransferTypeBulk {
				continue
			}
			// Endpoint zero is the device control endpoint, never an ADB bulk
			// transport endpoint.
			if endpoint.Address&usbEndpointNumberMask == 0 {
				continue
			}
			if endpoint.Address&usbEndpointDirectionIn != 0 {
				bulkInCount++
				endpointIn = endpoint.Address
			} else {
				bulkOutCount++
				endpointOut = endpoint.Address
			}
		}
		if bulkInCount != 1 || bulkOutCount != 1 {
			continue
		}
		candidates = append(candidates, usbADBCandidate{
			InterfaceNumber:  int(alternate.InterfaceNumber),
			AlternateSetting: int(alternate.AlternateSetting),
			EndpointIn:       endpointIn,
			EndpointOut:      endpointOut,
		})
	}

	switch len(candidates) {
	case 0:
		return usbADBCandidate{}, errUSBADBInterfaceNotFound
	case 1:
		return candidates[0], nil
	default:
		return usbADBCandidate{}, fmt.Errorf("%w: %d candidates", errUSBADBInterfaceAmbiguous, len(candidates))
	}
}
