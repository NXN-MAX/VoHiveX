//go:build darwin && cgo

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var location uint32
var transactionAT *usbAT
var keepAT bool

func closeAT() {
	if transactionAT != nil {
		transactionAT.Close()
		transactionAT = nil
	}
}

var voiceAT = regexp.MustCompile(`^(AT\+CLCC|ATA|ATH|ATD\+?[0-9]{3,24};|AT\+VTS="[0-9*#]"|AT\+CLDTMF=1,"[0-9*#,]{1,39}")$`)

func modemLocation() uint32 { return location }
func stateDir() string      { return os.Getenv("VOHIVEX_CALL_BRIDGE_STATE") }
func snapshotFile() string  { return filepath.Join(stateDir(), "usb-original.json") }
func lockFile(name string, exclusive bool) (*os.File, error) {
	f, e := os.OpenFile(filepath.Join(stateDir(), name), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	mode := syscall.LOCK_SH
	if exclusive {
		mode = syscall.LOCK_EX
	}
	if e = syscall.Flock(int(f.Fd()), lockMode(name, mode)); e != nil {
		f.Close()
		return nil, errors.New("音频接口正在使用或准备中，请稍后重试")
	}
	return f, nil
}
func lockMode(name string, mode int) int {
	if name == "at.lock" {
		return mode
	}
	return mode | syscall.LOCK_NB
}
func cancelled() error {
	select {
	case <-audioStop:
		return errors.New("音频准备已取消，将恢复接口")
	default:
		return nil
	}
}
func at(command string) (string, error) {
	l, e := lockFile("at.lock", true)
	if e != nil {
		return "", e
	}
	defer l.Close()
	a := transactionAT
	if a == nil {
		a, e = openDJIUSBATAtLocation(location)
		if e != nil {
			return "", e
		}
	}
	if keepAT {
		transactionAT = a
	} else {
		defer a.Close()
	}
	id := a.PhysicalIdentity()
	if id.VendorID != 0x2ca3 || id.ProductID != 0x4006 {
		closeAT()
		return "", errors.New("USB identity mismatch")
	}
	response, e := a.Command(command, 8*time.Second)
	if e != nil {
		closeAT()
	}
	return response, e
}
func checkedAT(command string) (string, error) {
	r, e := at(command)
	if e != nil {
		return r, e
	}
	if !atProbeSucceeded(r) || atResponseIsError(r) {
		return r, errors.New("modem rejected configuration command")
	}
	return r, nil
}
func usbConfig() (string, error) {
	r, e := checkedAT(`AT+QCFG="usbcfg"`)
	if e != nil {
		return "", e
	}
	for _, l := range strings.Split(r, "\n") {
		l = strings.TrimSpace(l)
		if strings.HasPrefix(l, `+QCFG: "usbcfg",`) {
			return strings.TrimPrefix(l, `+QCFG: `), nil
		}
	}
	return "", errors.New("USB configuration absent")
}

const originalUSB = `"usbcfg",0x2CA3,0x4006,1,1,1,1,1,0,0`
const enabledUSB = `"usbcfg",0x2CA3,0x4006,1,1,1,1,1,1,1`

type savedUSB struct {
	Location uint32    `json:"location"`
	Original string    `json:"original"`
	Created  time.Time `json:"created"`
}

func saved() (savedUSB, error) {
	var s savedUSB
	b, e := os.ReadFile(snapshotFile())
	if e == nil {
		e = json.Unmarshal(b, &s)
	}
	if e == nil && (s.Location != location || s.Original != originalUSB) {
		e = errors.New("rollback snapshot does not match this modem")
	}
	return s, e
}
func waitUSB(want string) error {
	closeAT()
	for deadline := time.Now().Add(45 * time.Second); time.Now().Before(deadline); {
		time.Sleep(time.Second)
		s, e := usbConfig()
		if e == nil && s == want {
			return nil
		}
	}
	return errors.New("USB did not return with the expected configuration; reconnect the modem and use restore")
}
func authorize() error {
	r, e := checkedAT("AT+QADBKEY?")
	if e != nil {
		return e
	}
	challenge, e := parseQADBChallenge(r)
	if e != nil {
		return e
	}
	key, e := deriveQADBKey(challenge)
	if e != nil {
		return e
	}
	_, e = checkedAT(`AT+QADBKEY="` + key + `"`)
	return e
}
func isReady() bool {
	if _, e := saved(); e != nil {
		return false
	}
	a, e := openDJIUSBADB(location)
	if e != nil {
		return false
	}
	defer a.Close()
	_, s, e := a.shellChecked(runtimeRouteReady(), 3*time.Second)
	return e == nil && s == 0
}
func prepare() (err error) {
	if isReady() {
		return nil
	}
	if _, e := saved(); e == nil {
		return errors.New("上一次音频准备未完成，请先恢复接口，再重新准备")
	}
	if err = runtimeCheckIdle(); err != nil {
		return err
	}
	var config string
	config, err = usbConfig()
	if err != nil {
		return err
	}
	if config != originalUSB {
		return errors.New("USB 接口由其他程序配置，未接管；请先恢复原始接口")
	}
	// Validate inputs before any persistent USB configuration write.
	if err = verifyRuntimeFiles(); err != nil {
		return err
	}
	b, _ := json.Marshal(savedUSB{location, config, time.Now()})
	f, e := os.OpenFile(snapshotFile(), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	_, e = f.Write(b)
	if e == nil {
		e = f.Sync()
	}
	f.Close()
	if e != nil {
		os.Remove(snapshotFile())
		return e
	}
	defer func() {
		if err != nil {
			if rollback := restore(); rollback != nil {
				err = fmt.Errorf("%v; 恢复尚未完成: %v", err, rollback)
			}
		}
	}()
	if err = cancelled(); err != nil {
		return err
	}
	if err = authorize(); err != nil {
		return err
	}
	if _, err = checkedAT("AT+QCFG=" + enabledUSB); err != nil {
		return err
	}
	if err = waitUSB(enabledUSB); err != nil {
		return err
	}
	_, _ = at("AT+CFUN=1,1")
	if err = waitUSB(enabledUSB); err != nil {
		return err
	}
	for _, step := range []string{"runtime-stage", "runtime-load-apr", "runtime-load-voice"} {
		if err = cancelled(); err != nil {
			return err
		}
		if err = runtimeStep(step); err != nil {
			return err
		}
	}
	// mdev publishes the audio nodes asynchronously after insmod.
	ready := false
	for i := 0; i < 20; i++ {
		a, e := openDJIUSBADB(location)
		if e == nil {
			_, s, e := a.shellChecked(runtimeDevices, 3*time.Second)
			a.Close()
			ready = e == nil && s == 0
		}
		if ready {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	if !ready {
		return errors.New("native PCM devices did not appear")
	}
	for _, step := range []string{"runtime-calibrate", "runtime-helper-check", "runtime-route"} {
		if err = cancelled(); err != nil {
			return err
		}
		if err = runtimeStep(step); err != nil {
			return err
		}
	}
	for i := 0; i < 30; i++ {
		if isReady() {
			return nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	return errors.New("VoLTE audio route did not become ready")
}
func restore() error {
	s, e := saved()
	if os.IsNotExist(e) {
		return nil
	}
	if e != nil {
		return e
	}
	if e = runtimeCheckIdle(); e != nil {
		return e
	}
	if isReady() {
		if e = runtimeStep("runtime-stop"); e != nil {
			return e
		}
	}
	current, e := usbConfig()
	if e != nil {
		return e
	}
	if current != s.Original && current != enabledUSB && current != `"usbcfg",0x2CA3,0x4006,1,1,1,1,1,0,1` {
		return errors.New("unexpected USB configuration; refusing to overwrite")
	}
	if _, e = checkedAT("AT+QCFG=" + s.Original); e != nil {
		return e
	}
	if e = waitUSB(s.Original); e != nil {
		return e
	}
	_, _ = at("AT+CFUN=1,1")
	if e = waitUSB(s.Original); e != nil {
		return e
	}
	return os.Remove(snapshotFile())
}
func run() error {
	n, e := strconv.ParseUint(os.Getenv("VOHIVEX_USB_LOCATION"), 0, 32)
	if e != nil || n == 0 {
		return errors.New("VOHIVEX_USB_LOCATION is required")
	}
	location = uint32(n)
	if !filepath.IsAbs(stateDir()) {
		return errors.New("VOHIVEX_CALL_BRIDGE_STATE must be an absolute private directory")
	}
	if e = os.MkdirAll(stateDir(), 0700); e != nil {
		return e
	}
	if len(os.Args) < 2 {
		return errors.New("expected status, prepare, restore, at, or stream")
	}
	op := os.Args[1]
	keepAT = op == "prepare" || op == "restore"
	defer closeAT()
	l, e := lockFile("operation.lock", op == "prepare" || op == "restore")
	if e != nil {
		return e
	}
	defer l.Close()
	switch op {
	case "at":
		if len(os.Args) != 3 || !voiceAT.MatchString(os.Args[2]) {
			return errors.New("unsupported voice command")
		}
		r, e := at(os.Args[2])
		if e != nil {
			return e
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"response": r})
	case "status":
		_, e := at("AT+CLCC")
		_, savedErr := saved()
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"connected": e == nil, "ready": e == nil && isReady(), "restore_required": savedErr == nil, "sample_rate": 8000})
	case "prepare":
		if e = prepare(); e != nil {
			return e
		}
	case "restore":
		if e = restore(); e != nil {
			return e
		}
	case "stream":
		if !isReady() {
			return errors.New("audio route is not prepared")
		}
		s, e := lockFile("stream.lock", true)
		if e != nil {
			return e
		}
		defer s.Close()
		return streamAudio(location)
	default:
		return errors.New("unknown bridge operation")
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"ok": true})
}
func main() {
	syscall.Umask(0077)
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT)
	go func() { <-signals; stopAudio() /* no implicit hangup or USB reset */ }()
	if e := run(); e != nil {
		out := os.Stdout
		if len(os.Args) > 1 && os.Args[1] == "stream" {
			out = os.Stderr
		}
		_ = json.NewEncoder(out).Encode(map[string]any{"error": e.Error()})
		os.Exit(1)
	}
}
