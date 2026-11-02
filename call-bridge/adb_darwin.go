//go:build darwin && cgo

package main

/*
#cgo pkg-config: libusb-1.0
#include <stdlib.h>
#include <libusb.h>
*/
import "C"

import (
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"unsafe"
)

// adbClient speaks the ADB wire protocol only to a standard Android USB ADB
// interface. The caller binds the handle to an exact supported VID/PID and
// physical location; descriptor selection below independently requires the
// ADB class triple and a unique bulk endpoint in each direction.
type adbClient struct {
	ctx              *C.libusb_context
	handle           *C.libusb_device_handle
	iface            int
	endpointIn       byte
	endpointOut      byte
	mu               sync.Mutex
	remoteMaxPayload int
	nextLocalID      uint32
	connected        bool
}

type adbStream struct {
	localID  uint32
	remoteID uint32
}

const (
	adbMaxPayload = 4096
	adbVersion    = 0x01000001
)

var (
	errADBAuthRequired = errors.New("模块 ADB 要求认证，无法自动控制通话组件")
	errADBTimeout      = errors.New("adb timeout")
)

func openDJIUSBADB(locationID uint32) (*adbClient, error) {
	if locationID == 0 {
		return nil, errors.New("USB ADB locationID must be non-zero")
	}
	var ctx *C.libusb_context
	if rc := C.libusb_init(&ctx); rc != 0 {
		return nil, fmt.Errorf("libusb init: %s", usbErrorName(rc))
	}
	handle, identity := openSupportedUSBModuleDeviceAtLocation(ctx, locationID)
	if handle == nil {
		C.libusb_exit(ctx)
		return nil, fmt.Errorf("DJI/Quectel USB ADB device at location 0x%08x not found", locationID)
	}
	dev := C.libusb_get_device(handle)
	if dev == nil {
		C.libusb_close(handle)
		C.libusb_exit(ctx)
		return nil, errors.New("libusb device handle has no device")
	}
	var config *C.struct_libusb_config_descriptor
	if rc := C.libusb_get_active_config_descriptor(dev, &config); rc != 0 {
		C.libusb_close(handle)
		C.libusb_exit(ctx)
		return nil, fmt.Errorf("get active USB config descriptor: %s", usbErrorName(rc))
	}
	defer C.libusb_free_config_descriptor(config)

	var alternates []usbADBAlternateDescriptor
	var descriptors []string
	interfaces := unsafe.Slice(config._interface, int(config.bNumInterfaces))
	for _, intf := range interfaces {
		altsettings := unsafe.Slice(intf.altsetting, int(intf.num_altsetting))
		for _, alt := range altsettings {
			endpoints := unsafe.Slice(alt.endpoint, int(alt.bNumEndpoints))
			endpointDescriptors := make([]usbADBEndpointDescriptor, 0, len(endpoints))
			endpointDetails := make([]string, 0, len(endpoints))
			for _, endpoint := range endpoints {
				address := byte(endpoint.bEndpointAddress)
				attributes := byte(endpoint.bmAttributes)
				endpointDescriptors = append(endpointDescriptors, usbADBEndpointDescriptor{
					Address:    address,
					Attributes: attributes,
				})
				endpointDetails = append(endpointDetails, fmt.Sprintf(
					"%02x/type%d",
					address,
					attributes&usbEndpointTransferTypeMask,
				))
			}
			alternates = append(alternates, usbADBAlternateDescriptor{
				InterfaceNumber:  byte(alt.bInterfaceNumber),
				AlternateSetting: byte(alt.bAlternateSetting),
				Class:            byte(alt.bInterfaceClass),
				Subclass:         byte(alt.bInterfaceSubClass),
				Protocol:         byte(alt.bInterfaceProtocol),
				Endpoints:        endpointDescriptors,
			})
			descriptors = append(descriptors, fmt.Sprintf(
				"if=%d alt=%d class=%02x subclass=%02x protocol=%02x endpoints=[%s]",
				byte(alt.bInterfaceNumber), byte(alt.bAlternateSetting), byte(alt.bInterfaceClass),
				byte(alt.bInterfaceSubClass), byte(alt.bInterfaceProtocol),
				strings.Join(endpointDetails, ","),
			))
		}
	}
	target, selectionErr := selectQDC507ADBInterface(alternates)
	if selectionErr != nil {
		C.libusb_close(handle)
		C.libusb_exit(ctx)
		return nil, fmt.Errorf(
			"standard USB ADB interface unavailable on %s: %w; requires alt 0 class ff/subclass 42/protocol 01 with exactly one bulk IN and one bulk OUT endpoint (active descriptors: %s)",
			identity.String(), selectionErr, strings.Join(descriptors, "; "),
		)
	}
	if rc := C.libusb_claim_interface(handle, C.int(target.InterfaceNumber)); rc != 0 {
		C.libusb_close(handle)
		C.libusb_exit(ctx)
		return nil, fmt.Errorf("claim USB ADB interface %d: %s", target.InterfaceNumber, usbErrorName(rc))
	}
	return &adbClient{
		ctx:              ctx,
		handle:           handle,
		iface:            target.InterfaceNumber,
		endpointIn:       target.EndpointIn,
		endpointOut:      target.EndpointOut,
		remoteMaxPayload: adbMaxPayload,
		nextLocalID:      1,
	}, nil
}

func (a *adbClient) Close() {
	if a == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.handle == nil {
		return
	}
	C.libusb_release_interface(a.handle, C.int(a.iface))
	C.libusb_close(a.handle)
	C.libusb_exit(a.ctx)
	a.handle = nil
	a.ctx = nil
}

func (a *adbClient) isOpen() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a != nil && a.handle != nil
}

func (a *adbClient) bulkWrite(payload []byte, timeout time.Duration) error {
	if len(payload) == 0 {
		return nil
	}
	var transferred C.int
	rc := C.libusb_bulk_transfer(
		a.handle,
		C.uchar(a.endpointOut),
		(*C.uchar)(unsafe.Pointer(&payload[0])),
		C.int(len(payload)),
		&transferred,
		C.uint(timeout.Milliseconds()),
	)
	if rc != 0 {
		return fmt.Errorf("USB ADB bulk write: %s", usbErrorName(rc))
	}
	if int(transferred) != len(payload) {
		return fmt.Errorf("USB ADB bulk write short transfer: %d/%d", int(transferred), len(payload))
	}
	return nil
}

func (a *adbClient) bulkRead(buf []byte, timeout time.Duration) (int, error) {
	var transferred C.int
	rc := C.libusb_bulk_transfer(
		a.handle,
		C.uchar(a.endpointIn),
		(*C.uchar)(unsafe.Pointer(&buf[0])),
		C.int(len(buf)),
		&transferred,
		C.uint(timeout.Milliseconds()),
	)
	if rc != 0 {
		if rc == C.LIBUSB_ERROR_TIMEOUT {
			return 0, errADBTimeout
		}
		return 0, fmt.Errorf("USB ADB bulk read: %s", usbErrorName(rc))
	}
	return int(transferred), nil
}

// ---- ADB wire protocol ----

func adbCommand(text string) uint32 {
	b := []byte(text)
	return uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24
}

func adbChecksum(payload []byte) uint32 {
	var sum uint32
	for _, b := range payload {
		sum += uint32(b)
	}
	return sum
}

func adbEncodeHeader(cmd, arg0, arg1 uint32, payload []byte) []byte {
	h := make([]byte, 24)
	lePutUint32(h[0:], cmd)
	lePutUint32(h[4:], arg0)
	lePutUint32(h[8:], arg1)
	lePutUint32(h[12:], uint32(len(payload)))
	lePutUint32(h[16:], adbChecksum(payload))
	lePutUint32(h[20:], cmd^0xffffffff)
	return h
}

func lePutUint32(b []byte, v uint32) {
	b[0] = byte(v)
	b[1] = byte(v >> 8)
	b[2] = byte(v >> 16)
	b[3] = byte(v >> 24)
}

func leUint32(b []byte) uint32 {
	return uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24
}

type adbMessage struct {
	command uint32
	arg0    uint32
	arg1    uint32
	payload []byte
}

func (a *adbClient) sendLocked(cmd, arg0, arg1 uint32, payload []byte, timeout time.Duration) error {
	if err := a.bulkWrite(adbEncodeHeader(cmd, arg0, arg1, payload), timeout); err != nil {
		return err
	}
	if len(payload) > 0 {
		return a.bulkWrite(payload, timeout)
	}
	return nil
}

func (a *adbClient) readExactlyLocked(n int, deadline time.Time) ([]byte, error) {
	if n == 0 {
		return []byte{}, nil
	}
	out := make([]byte, 0, n)
	buf := make([]byte, 512)
	for len(out) < n && time.Now().Before(deadline) {
		remaining := time.Until(deadline)
		if remaining > 100*time.Millisecond {
			remaining = 100 * time.Millisecond
		}
		got, err := a.bulkRead(buf, remaining)
		if err != nil {
			if errors.Is(err, errADBTimeout) {
				continue
			}
			return nil, err
		}
		if got == 0 {
			continue
		}
		need := n - len(out)
		if got > need {
			got = need
		}
		out = append(out, buf[:got]...)
	}
	if len(out) != n {
		return nil, fmt.Errorf("等待模块 ADB 数据超时（需要 %d 字节，得到 %d）", n, len(out))
	}
	return out, nil
}

func (a *adbClient) receiveLocked(deadline time.Time) (adbMessage, error) {
	header, err := a.readExactlyLocked(24, deadline)
	if err != nil {
		return adbMessage{}, err
	}
	length := leUint32(header[12:])
	if length > adbMaxPayload {
		return adbMessage{}, fmt.Errorf("ADB 消息长度无效: %d", length)
	}
	payload, err := a.readExactlyLocked(int(length), deadline)
	if err != nil {
		return adbMessage{}, err
	}
	if adbChecksum(payload) != leUint32(header[16:]) {
		return adbMessage{}, errors.New("ADB 消息校验和不匹配")
	}
	return adbMessage{
		command: leUint32(header[0:]),
		arg0:    leUint32(header[4:]),
		arg1:    leUint32(header[8:]),
		payload: payload,
	}, nil
}

func (a *adbClient) connectLocked() error {
	if a.connected {
		return nil
	}
	const (
		cmdCNXN = 0x4e584e43 // "CNXN"
		cmdAUTH = 0x48545541 // "AUTH"
		cmdWRTE = 0x45545257 // "WRTE"
		cmdOKAY = 0x59414b4f // "OKAY"
		cmdCLSE = 0x45534c43 // "CLSE"
	)
	banner := append([]byte("host::MaVo"), 0)
	sendConnect := func() error {
		return a.sendLocked(cmdCNXN, adbVersion, adbMaxPayload, banner, 2*time.Second)
	}
	if err := sendConnect(); err != nil {
		return err
	}
	deadline := time.Now().Add(8 * time.Second)
	stale := 0
	for time.Now().Before(deadline) {
		msg, err := a.receiveLocked(deadline)
		if err != nil {
			return err
		}
		switch msg.command {
		case cmdAUTH:
			return errADBAuthRequired
		case cmdCNXN:
			if msg.arg1 > 0 {
				if int(msg.arg1) < a.remoteMaxPayload {
					a.remoteMaxPayload = int(msg.arg1)
				}
				a.connected = true
				return nil
			}
		case cmdWRTE, cmdOKAY, cmdCLSE:
			stale++
			if stale > 64 {
				return errors.New("ADB 旧流无法清理")
			}
			if msg.arg0 != 0 && msg.arg1 != 0 {
				if err := a.sendLocked(cmdCLSE, msg.arg1, msg.arg0, nil, 2*time.Second); err != nil {
					return err
				}
			}
			if err := sendConnect(); err != nil {
				return err
			}
			continue
		default:
			return fmt.Errorf("模块未接受 ADB CNXN（command=0x%08X）", msg.command)
		}
	}
	return fmt.Errorf("等待模块接受 ADB CNXN 超时")
}

func (a *adbClient) openServiceLocked(service string) (adbStream, error) {
	const (
		cmdCNXN = 0x4e584e43
		cmdOKAY = 0x59414b4f
		cmdOPEN = 0x4e45504f // "OPEN"
		cmdWRTE = 0x45545257
		cmdCLSE = 0x45534c43
	)
	payload := append([]byte(service), 0)
	if len(payload) > a.remoteMaxPayload {
		return adbStream{}, errors.New("ADB 服务命令过长")
	}
	localID := a.nextLocalID
	a.nextLocalID++
	if a.nextLocalID == 0 {
		a.nextLocalID = 1
	}
	if err := a.sendLocked(cmdOPEN, localID, 0, payload, 2*time.Second); err != nil {
		return adbStream{}, err
	}
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		msg, err := a.receiveLocked(deadline)
		if err != nil {
			return adbStream{}, err
		}
		switch msg.command {
		case cmdOKAY:
			if msg.arg0 != 0 && msg.arg1 == localID && len(msg.payload) == 0 {
				return adbStream{localID: localID, remoteID: msg.arg0}, nil
			}
		case cmdCNXN:
			if msg.arg1 > 0 {
				if int(msg.arg1) < a.remoteMaxPayload {
					a.remoteMaxPayload = int(msg.arg1)
				}
				continue
			}
		case cmdCLSE:
			if msg.arg1 == localID {
				return adbStream{}, errors.New("模块拒绝 ADB 服务")
			}
			if msg.arg0 != 0 && msg.arg1 != 0 {
				if err := a.sendLocked(cmdCLSE, msg.arg1, msg.arg0, nil, 2*time.Second); err != nil {
					return adbStream{}, err
				}
			}
			continue
		case cmdWRTE:
			if msg.arg0 != 0 && msg.arg1 != 0 {
				if err := a.sendLocked(cmdCLSE, msg.arg1, msg.arg0, nil, 2*time.Second); err != nil {
					return adbStream{}, err
				}
			}
			continue
		default:
			return adbStream{}, errors.New("模块拒绝 ADB 服务")
		}
	}
	return adbStream{}, fmt.Errorf("等待模块打开 ADB 服务超时")
}

func (a *adbClient) writeStreamLocked(stream adbStream, data []byte, timeout time.Duration) error {
	const (
		cmdWRTE = 0x45545257
		cmdOKAY = 0x59414b4f
	)
	if len(data) > a.remoteMaxPayload {
		return errors.New("ADB sync 数据块过大")
	}
	if err := a.sendLocked(cmdWRTE, stream.localID, stream.remoteID, data, timeout); err != nil {
		return err
	}
	deadline := time.Now().Add(10 * time.Second)
	msg, err := a.receiveLocked(deadline)
	if err != nil {
		return err
	}
	if msg.command != cmdOKAY || msg.arg0 != stream.remoteID || msg.arg1 != stream.localID || len(msg.payload) != 0 {
		return errors.New("模块未确认 ADB 数据块")
	}
	return nil
}

func (a *adbClient) closeStreamLocked(stream adbStream) error {
	const (
		cmdCLSE = 0x45534c43
		cmdWRTE = 0x45545257
		cmdOKAY = 0x59414b4f
	)
	if err := a.sendLocked(cmdCLSE, stream.localID, stream.remoteID, nil, 2*time.Second); err != nil {
		return err
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		msg, err := a.receiveLocked(deadline)
		if err != nil {
			return err
		}
		if msg.command == cmdCLSE && msg.arg0 == stream.remoteID && msg.arg1 == stream.localID && len(msg.payload) == 0 {
			return nil
		}
		if msg.command == cmdWRTE && msg.arg0 == stream.remoteID && msg.arg1 == stream.localID {
			if err := a.sendLocked(cmdOKAY, stream.localID, stream.remoteID, nil, 2*time.Second); err != nil {
				return err
			}
			continue
		}
		return errors.New("ADB sync 关闭响应无效")
	}
	return fmt.Errorf("等待模块关闭 ADB sync 流超时")
}

// shellChecked runs a shell command over ADB and returns output plus exit status.
func (a *adbClient) shellChecked(command string, timeout time.Duration) (string, int, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.handle == nil {
		return "", 0, errors.New("ADB 通道未打开")
	}
	if err := a.connectLocked(); err != nil {
		return "", 0, err
	}
	token := adbToken()
	// A normal false probe uses `exit 1`; keep that exit inside a subshell so
	// the outer ADB shell can still print the machine-readable status marker.
	wrapped := "( " + command + "; ); __mavo_status=$?; printf '\\n__MAVO_STATUS_" + token + "_%u__\\n' \"$__mavo_status\""
	stream, err := a.openServiceLocked("shell:" + wrapped)
	if err != nil {
		return "", 0, err
	}
	var output strings.Builder
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		msg, err := a.receiveLocked(deadline)
		if err != nil {
			a.connected = false
			_ = a.closeStreamLocked(stream)
			return output.String(), 0, err
		}
		switch msg.command {
		case 0x45545257: // WRTE
			if msg.arg0 == stream.remoteID && msg.arg1 == stream.localID {
				output.Write(msg.payload)
				if err := a.sendLocked(0x59414b4f, stream.localID, stream.remoteID, nil, 2*time.Second); err != nil { // OKAY
					return output.String(), 0, err
				}
			}
		case 0x45534c43: // CLSE
			if msg.arg1 != stream.localID {
				continue
			}
			if msg.arg0 != 0 {
				_ = a.sendLocked(0x45534c43, stream.localID, stream.remoteID, nil, 2*time.Second)
			}
			status, ok := parseADBStatus(output.String(), token)
			if !ok {
				a.connected = false
				return output.String(), 0, errors.New("模块 shell 没有返回退出状态")
			}
			return output.String(), status, nil
		}
	}
	a.connected = false
	_ = a.closeStreamLocked(stream)
	return output.String(), 0, fmt.Errorf("等待模块 shell 超时")
}

func adbToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return fmt.Sprintf("%x", b)
}

func parseADBStatus(raw, token string) (int, bool) {
	prefix := "__MAVO_STATUS_" + token + "_"
	idx := strings.LastIndex(raw, prefix)
	if idx == -1 {
		return 0, false
	}
	rest := raw[idx+len(prefix):]
	end := strings.Index(rest, "__")
	if end == -1 {
		return 0, false
	}
	var status int
	if _, err := fmt.Sscanf(rest[:end], "%d", &status); err != nil {
		return 0, false
	}
	return status, true
}

// push copies data to a remote path over the ADB sync service.
func (a *adbClient) push(data []byte, remotePath string, mode uint32, timeout time.Duration) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.handle == nil {
		return errors.New("ADB 通道未打开")
	}
	if strings.ContainsAny(remotePath, ",\x00") {
		return errors.New("ADB push 目标路径无效")
	}
	if err := a.connectLocked(); err != nil {
		return err
	}
	stream, err := a.openServiceLocked("sync:")
	if err != nil {
		return err
	}
	sendName := []byte(fmt.Sprintf("%s,%d", remotePath, mode))
	if err := a.writeSyncLocked(stream, "SEND", sendName, timeout); err != nil {
		_ = a.closeStreamLocked(stream)
		return err
	}
	chunkCapacity := adbMaxPayload - 8
	for offset := 0; offset < len(data); offset += chunkCapacity {
		end := offset + chunkCapacity
		if end > len(data) {
			end = len(data)
		}
		if err := a.writeSyncLocked(stream, "DATA", data[offset:end], timeout); err != nil {
			_ = a.closeStreamLocked(stream)
			return err
		}
	}
	done := make([]byte, 4)
	lePutUint32(done, uint32(time.Now().Unix()))
	if err := a.writeSyncLocked(stream, "DONE", done, timeout); err != nil {
		_ = a.closeStreamLocked(stream)
		return err
	}
	var response []byte
	deadline := time.Now().Add(20 * time.Second)
	for len(response) < 8 && time.Now().Before(deadline) {
		msg, err := a.receiveLocked(deadline)
		if err != nil {
			_ = a.closeStreamLocked(stream)
			return err
		}
		if msg.command == 0x45545257 && msg.arg0 == stream.remoteID && msg.arg1 == stream.localID { // WRTE
			response = append(response, msg.payload...)
			if err := a.sendLocked(0x59414b4f, stream.localID, stream.remoteID, nil, 2*time.Second); err != nil { // OKAY
				return err
			}
		} else if msg.command == 0x45534c43 { // CLSE
			return errors.New("ADB sync 提前关闭")
		}
	}
	if len(response) < 8 {
		_ = a.closeStreamLocked(stream)
		return fmt.Errorf("等待模块 ADB sync 响应超时")
	}
	id := string(response[:4])
	value := leUint32(response[4:8])
	if id == "FAIL" {
		detail := ""
		if int(value) > 0 && len(response) >= 8+int(value) {
			detail = string(response[8 : 8+value])
		}
		_ = a.closeStreamLocked(stream)
		return fmt.Errorf("模块拒绝文件传输：%s", detail)
	}
	if id != "OKAY" || value != 0 {
		_ = a.closeStreamLocked(stream)
		return errors.New("ADB sync 返回无效状态")
	}
	return a.closeStreamLocked(stream)
}

func (a *adbClient) writeSyncLocked(stream adbStream, id string, payload []byte, timeout time.Duration) error {
	packet := make([]byte, 8+len(payload))
	copy(packet[:4], id)
	lePutUint32(packet[4:8], uint32(len(payload)))
	copy(packet[8:], payload)
	return a.writeStreamLocked(stream, packet, timeout)
}
