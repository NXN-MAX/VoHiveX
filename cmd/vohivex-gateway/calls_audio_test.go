package main

import (
	"encoding/binary"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func testPCM(t *testing.T, path string, samples int) {
	t.Helper()
	b := make([]byte, 44+samples*2)
	copy(b, "RIFF")
	binary.LittleEndian.PutUint32(b[4:], uint32(len(b)-8))
	copy(b[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(b[16:], 16)
	binary.LittleEndian.PutUint16(b[20:], 1)
	binary.LittleEndian.PutUint16(b[22:], 1)
	binary.LittleEndian.PutUint32(b[24:], 8000)
	binary.LittleEndian.PutUint32(b[28:], 16000)
	binary.LittleEndian.PutUint16(b[32:], 2)
	binary.LittleEndian.PutUint16(b[34:], 16)
	copy(b[36:], "data")
	binary.LittleEndian.PutUint32(b[40:], uint32(samples*2))
	for n := 44; n < len(b); n += 2 {
		binary.LittleEndian.PutUint16(b[n:], uint16((n%160)*100))
	}
	if err := os.WriteFile(path, b, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestWAVRequiresSamplesAndUsesAudioDuration(t *testing.T) {
	p := filepath.Join(t.TempDir(), "test.wav")
	testPCM(t, p, 16000)
	if duration, err := wavDuration(p); err != nil || duration != 2 {
		t.Fatal(duration, err)
	}
	testPCM(t, p, 0)
	if _, err := wavDuration(p); err == nil {
		t.Fatal("empty WAV accepted")
	}
	testPCM(t, p, 8000)
	data, _ := os.ReadFile(p)
	for _, broken := range [][]byte{data[:len(data)-2], append([]byte("NOT!"), data[4:]...), data[:20]} {
		_ = os.WriteFile(p, broken, 0600)
		if _, err := wavDuration(p); err == nil {
			t.Fatal("invalid WAV accepted")
		}
	}
}

func activeRecordingTestManager(t *testing.T) (*callManager, callHistoryItem) {
	t.Helper()
	m, err := newCallManager(":memory:", filepath.Join(t.TempDir(), "recordings"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Close() })
	call, err := m.createOutgoing(map[string]any{"id": "demo", "name": "Test modem"}, "+12025550123")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.db.Exec(`UPDATE call_history SET state='active' WHERE id=?`, call.ID); err != nil {
		t.Fatal(err)
	}
	call.State = "active"
	return m, call
}

func TestRecorderStartupFailureDoesNotReportActive(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "arecord"), []byte("#!/bin/sh\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("VOHIVEX_CALL_AUDIO_DEVICE", "null")
	m, call := activeRecordingTestManager(t)
	if _, err := m.startRecording(call.ID); err == nil {
		t.Fatal("failed capture reported success")
	}
	if state := m.recordingState(); state["active"] != false || state["error"] == nil {
		t.Fatal(state)
	}
	files, _ := os.ReadDir(m.recordingsDir)
	if len(files) != 0 {
		t.Fatal("failed recording left a file")
	}
}

func TestRecorderWithoutPCMSamplesDoesNotReportActive(t *testing.T) {
	dir := t.TempDir()
	// A USB audio endpoint can enumerate while its modem-side PCM route is
	// missing. Keep the process alive without writing samples to reproduce it.
	if err := os.WriteFile(filepath.Join(dir, "arecord"), []byte("#!/bin/sh\nfor target do :; done\n: > \"$target\"\nexec /bin/sleep 30\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("VOHIVEX_CALL_AUDIO_DEVICE", "null")
	m, call := activeRecordingTestManager(t)
	if _, err := m.startRecording(call.ID); err == nil {
		t.Fatal("capture without samples reported active")
	}
	if state := m.recordingState(); state["active"] != false || state["error"] == nil {
		t.Fatal(state)
	}
	files, _ := os.ReadDir(m.recordingsDir)
	if len(files) != 0 {
		t.Fatal("failed capture left partial audio")
	}
}

func TestRecorderReceivesPCMSamplesBeforeReportingActive(t *testing.T) {
	dir := t.TempDir()
	fixture := filepath.Join(dir, "source.wav")
	testPCM(t, fixture, 8000)
	if err := os.WriteFile(filepath.Join(dir, "arecord"), []byte("#!/bin/sh\nfor target do :; done\n/bin/cat \"$VOHIVEX_TEST_PCM\" > \"$target\"\nexec /bin/sleep 30\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("VOHIVEX_TEST_PCM", fixture)
	t.Setenv("VOHIVEX_CALL_AUDIO_DEVICE", "null")
	m, call := activeRecordingTestManager(t)
	state, err := m.startRecording(call.ID)
	if err != nil || state["active"] != true {
		t.Fatal(state, err)
	}
	info, err := os.Stat(m.recorder.tempPath)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("live recording is not private", info, err)
	}
	item, err := m.stopRecording()
	if err != nil || item.DurationSeconds != 1 {
		t.Fatal(item, err)
	}
}

func TestCaptureHasSamplesSupportsUnfinishedWAV(t *testing.T) {
	p := filepath.Join(t.TempDir(), "live.wav")
	testPCM(t, p, 1600)
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	// arecord reserves sizes before the final header is written on stop.
	binary.LittleEndian.PutUint32(data[4:], 0x7ffff000)
	binary.LittleEndian.PutUint32(data[40:], 0x7ffff000)
	if err := os.WriteFile(p, data, 0600); err != nil {
		t.Fatal(err)
	}
	if !captureHasSamples(p) {
		t.Fatal("live samples rejected because header is not finalized")
	}
	if _, err := wavDuration(p); err == nil {
		t.Fatal("unfinished header accepted for archive")
	}
	for _, size := range []int{12, 28, 44, 100} {
		if err := os.WriteFile(p, data[:size], 0600); err != nil {
			t.Fatal(err)
		}
		if captureHasSamples(p) {
			t.Fatalf("insufficient capture bytes accepted: %d", size)
		}
	}
	// An unrelated format must not qualify as our call PCM stream.
	binary.LittleEndian.PutUint16(data[22:], 2)
	_ = os.WriteFile(p, data, 0600)
	if captureHasSamples(p) {
		t.Fatal("unexpected channel count accepted")
	}
	// Silence is valid capture data. It is not evidence of audible call media.
	testPCM(t, p, 1600)
	data, _ = os.ReadFile(p)
	clear(data[44:])
	_ = os.WriteFile(p, data, 0600)
	if !captureHasSamples(p) {
		t.Fatal("silence incorrectly rejected as missing data")
	}
}

func TestExitedRecorderSavesValidAudioAndClearsActive(t *testing.T) {
	m, call := activeRecordingTestManager(t)
	temp := filepath.Join(m.recordingsDir, ".test.part.wav")
	testPCM(t, temp, 16000)
	done := make(chan struct{})
	close(done)
	m.recorder = &activeCallRecorder{id: "audio-test", call: call, startedAt: time.Now().Add(-time.Minute), tempPath: temp, finalPath: filepath.Join(m.recordingsDir, "test.wav"), cmd: &exec.Cmd{}, done: done}
	state := m.recordingState()
	if state["active"] != false {
		t.Fatal("exited recorder still active")
	}
	items, err := m.recordings()
	if err != nil || len(items) != 1 || items[0].DurationSeconds != 2 {
		t.Fatal(items, err)
	}
	updated, err := m.callByID(call.ID)
	if err != nil || updated.RecordingID != "audio-test" {
		t.Fatal(updated, err)
	}
	if _, err := m.stopRecording(); err == nil {
		t.Fatal("stopped recorder saved twice")
	}
}

func TestUSBIdentityDoesNotDependOnProcAsound(t *testing.T) {
	root := t.TempDir()
	usb := filepath.Join(root, "usb")
	card := filepath.Join(usb, "interface", "sound", "card2")
	_ = os.MkdirAll(card, 0700)
	_ = os.WriteFile(filepath.Join(usb, "idVendor"), []byte("2ca3\n"), 0600)
	_ = os.WriteFile(filepath.Join(usb, "idProduct"), []byte("4006\n"), 0600)
	link := filepath.Join(root, "card2")
	_ = os.Symlink(card, link)
	if got := usbSoundIdentity(link); got != "2ca3:4006" {
		t.Fatal(got)
	}
	if got := usbSoundIdentity(root); got != "" {
		t.Fatal("unrelated card identified as modem")
	}
}

func TestALSAContainerToolchainCapture(t *testing.T) {
	if os.Getenv("VOHIVEX_TEST_ALSA_CAPTURE") != "1" {
		t.Skip("isolated container-only ALSA test")
	}
	path := filepath.Join(t.TempDir(), "null.wav")
	// The null source is synthetic and needs no /dev or host microphone mount.
	cmd := exec.Command("arecord", "-q", "-D", "null", "-d", "1", "-t", "wav", "-f", "S16_LE", "-r", "8000", "-c", "1", path)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("ALSA capture failed: %v %s", err, output)
	}
	if duration, err := wavDuration(path); err != nil || duration != 1 {
		t.Fatal(duration, err)
	}
}

func TestRecorderCrashWithEmptyAudioReportsError(t *testing.T) {
	m, call := activeRecordingTestManager(t)
	path := filepath.Join(m.recordingsDir, ".empty.wav")
	testPCM(t, path, 0)
	done := make(chan struct{})
	close(done)
	m.recorder = &activeCallRecorder{id: "empty", call: call, tempPath: path, finalPath: path + ".final", cmd: &exec.Cmd{}, done: done}
	state := m.recordingState()
	if state["active"] != false || state["error"] == nil {
		t.Fatal(state)
	}
	items, err := m.recordings()
	if err != nil || len(items) != 0 {
		t.Fatal("empty audio was archived", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("empty partial audio retained")
	}
}

func TestVoWiFiAudioDoesNotUseHostALSA(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "arecord"), []byte("#!/bin/sh\nexit 0\n"), 0700)
	t.Setenv("PATH", dir)
	t.Setenv("VOHIVEX_CALL_AUDIO_DEVICE", "null")
	device := map[string]any{"id": "modem-1", "running": true, "vowifi_enabled": true}
	clcc := "OK"
	app, commands := callRouteTestApp(t, device, &clcc)
	w := routeRequest(app, "GET", "/api/calls/status?device_id=modem-1", "")
	var result map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &result)
	audio := configMap(result["audio"])
	if w.Code != 200 || audio["available"] != false || audio["source"] != "ims" || len(*commands) != 0 {
		t.Fatal(w.Code, audio, *commands)
	}
}
