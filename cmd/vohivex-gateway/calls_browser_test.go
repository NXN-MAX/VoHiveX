package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"golang.org/x/net/websocket"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBrowserAudioHelperProcess(t *testing.T) {
	if os.Getenv("VOHIVEX_FAKE_AUDIO_HELPER") != "1" {
		return
	}
	op := os.Args[len(os.Args)-1]
	if op == "status" {
		io.WriteString(os.Stdout, `{"connected":true,"ready":true,"restore_required":true}`)
		os.Exit(0)
	}
	if op != "stream" {
		os.Exit(17)
	} // No AT/hangup/config commands allowed in this test.
	frame := []byte{4, 0, 0, 0, 100, 0, 200, 0}
	_, _ = os.Stdout.Write(frame)
	for {
		h := make([]byte, 4)
		if _, e := io.ReadFull(os.Stdin, h); e != nil {
			os.Exit(0)
		}
		n := binary.LittleEndian.Uint32(h)
		if n > 3200 {
			os.Exit(18)
		}
		b := make([]byte, n)
		if _, e := io.ReadFull(os.Stdin, b); e != nil {
			os.Exit(0)
		}
		os.Stdout.Write(append(h, b...))
	}
}
func browserTestApp(t *testing.T) (*application, *httptest.Server) {
	t.Helper()
	dir := t.TempDir()
	executable, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	helper := filepath.Join(dir, "helper")
	script := "#!/bin/sh\nexport VOHIVEX_FAKE_AUDIO_HELPER=1\nexec '" + strings.ReplaceAll(executable, "'", "'\\''") + "' -test.run=^TestBrowserAudioHelperProcess$ -- \"$@\"\n"
	if e = os.WriteFile(helper, []byte(script), 0700); e != nil {
		t.Fatal(e)
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test" {
			w.WriteHeader(401)
			return
		}
		writeJSON(w, 200, map[string]any{"devices": []any{}})
	}))
	t.Cleanup(upstream.Close)
	manager, e := newCallManager(":memory:", filepath.Join(dir, "recordings"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { manager.Close() })
	app := &application{upstream: strings.TrimPrefix(upstream.URL, "http://"), media: newBrowserMedia(helper), calls: manager}
	server := httptest.NewServer(app.routes())
	t.Cleanup(server.Close)
	t.Cleanup(func() { app.media.Close(manager) })
	return app, server
}
func browserTicket(t *testing.T, app *application) string {
	t.Helper()
	r := httptest.NewRequest("POST", "/api/calls/audio/ticket", strings.NewReader(`{"device_id":"local-usb-modem"}`))
	r.Header.Set("Authorization", "Bearer test")
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	app.routes().ServeHTTP(w, r)
	if w.Code != 201 {
		t.Fatalf("ticket HTTP %d %s", w.Code, w.Body.String())
	}
	var result map[string]any
	json.Unmarshal(w.Body.Bytes(), &result)
	return result["ticket"].(string)
}
func TestBrowserAudioDuplexTicketAndDisconnect(t *testing.T) {
	app, server := browserTestApp(t)
	if ws, e := websocket.Dial(strings.Replace(server.URL, "http:", "ws:", 1)+"/api/calls/audio/stream", "", "http://attacker.invalid"); e == nil {
		ws.Close()
		t.Fatal("cross-origin accepted")
	}
	token := browserTicket(t, app)
	ws, e := websocket.Dial(strings.Replace(server.URL, "http:", "ws:", 1)+"/api/calls/audio/stream", "", server.URL)
	if e != nil {
		t.Fatal(e)
	}
	defer ws.Close()
	ws.SetDeadline(time.Now().Add(4 * time.Second))
	websocket.JSON.Send(ws, map[string]string{"ticket": token})
	var ready map[string]any
	if e = websocket.JSON.Receive(ws, &ready); e != nil || ready["ready"] != true {
		t.Fatalf("ready: %v %v", ready, e)
	}
	var received []byte
	if e = websocket.Message.Receive(ws, &received); e != nil || !bytes.Equal(received, []byte{100, 0, 200, 0}) {
		t.Fatalf("initial audio %v %v", received, e)
	}
	want := []byte{1, 0, 255, 127, 0, 128}
	websocket.Message.Send(ws, want)
	if e = websocket.Message.Receive(ws, &received); e != nil || !bytes.Equal(received, want) {
		t.Fatalf("duplex %v %v", received, e)
	}
	ws.Close()
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		app.media.mu.Lock()
		idle := app.media.active == nil
		app.media.mu.Unlock()
		if idle {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	app.media.mu.Lock()
	if app.media.active != nil || app.media.consumeTicket(token) {
		t.Fatal("connection was not released or ticket could be reused")
	}
	app.media.mu.Unlock()
	// A new connection can be issued after closing audio without any hangup command.
	browserTicket(t, app)
}
func TestBrowserAudioTicketAndRouteGuards(t *testing.T) {
	app, _ := browserTestApp(t)
	for _, tc := range []struct {
		token, device string
		code          int
	}{{"", nativeCallDevice, 401}, {"test", "another-device", 409}} {
		r := httptest.NewRequest("POST", "/api/calls/audio/ticket", strings.NewReader(`{"device_id":"`+tc.device+`"}`))
		r.Header.Set("Content-Type", "application/json")
		if tc.token != "" {
			r.Header.Set("Authorization", "Bearer "+tc.token)
		}
		w := httptest.NewRecorder()
		app.routes().ServeHTTP(w, r)
		if w.Code != tc.code {
			t.Fatalf("HTTP %d want %d", w.Code, tc.code)
		}
	}
	m := app.media
	m.tickets["expired"] = mediaTicket{nativeCallDevice, time.Now().Add(-time.Second)}
	m.tickets["wrong"] = mediaTicket{"other", time.Now().Add(time.Hour)}
	if m.consumeTicket("expired") || m.consumeTicket("wrong") || m.consumeTicket("missing") {
		t.Fatal("invalid ticket accepted")
	}
}
func TestBrowserRecordingStereoAndAutoFinish(t *testing.T) {
	app, _ := browserTestApp(t)
	m := app.media
	m.streaming = true
	active, e := app.calls.reconcile(map[string]any{"id": nativeCallDevice, "name": "USB"}, []parsedVoiceCall{{Index: 1, Direction: "outgoing", State: "active", Number: "+12025550123"}}, time.Now())
	if e != nil {
		t.Fatal(e)
	}
	if _, e = m.startRecording(active[0], app.calls.recordingsDir); e != nil {
		t.Fatal(e)
	}
	m.uplink = []byte{3, 0, 4, 0}
	m.recordDownlink([]byte{1, 0, 2, 0})
	_, e = app.calls.reconcile(map[string]any{"id": nativeCallDevice, "name": "USB"}, nil, time.Now().Add(time.Second))
	if e != nil {
		t.Fatal(e)
	}
	app.finishBrowserRecording()
	if m.recording != nil {
		t.Fatal("ended call still recording")
	}
	files, _ := filepath.Glob(filepath.Join(app.calls.recordingsDir, "*.wav"))
	if len(files) != 1 {
		t.Fatal(files)
	}
	b, _ := os.ReadFile(files[0])
	info, _ := os.Stat(files[0])
	if len(b) != 52 || string(b[:4]) != "RIFF" || binary.LittleEndian.Uint16(b[22:]) != 2 || !bytes.Equal(b[44:], []byte{1, 0, 3, 0, 2, 0, 4, 0}) || info.Mode().Perm() != 0600 {
		t.Fatal("invalid stereo WAV or permissions")
	}
	if _, e = m.startRecording(active[0], app.calls.recordingsDir); e != nil {
		t.Fatal(e)
	}
	if _, e = m.stopRecording(app.calls); e == nil {
		t.Fatal("empty recording accepted")
	}
}

// Opt-in hardware smoke test: use a prepared modem, without dialing, changing
// USB/SIM/radio settings or sending non-silent uplink. The auth fixture only
// supplies identity; the gateway routes and native capture/playback are real.
func TestBrowserAudioLiveCapture(t *testing.T) {
	helper := os.Getenv("VOHIVEX_LIVE_AUDIO_HELPER")
	if helper == "" {
		t.Skip("explicit prepared-hardware opt-in required")
	}
	app, server := browserTestApp(t)
	app.media = newBrowserMedia(helper)
	token := browserTicket(t, app)
	ws, e := websocket.Dial(strings.Replace(server.URL, "http:", "ws:", 1)+"/api/calls/audio/stream", "", server.URL)
	if e != nil {
		t.Fatal(e)
	}
	defer ws.Close()
	ws.SetDeadline(time.Now().Add(10 * time.Second))
	websocket.JSON.Send(ws, map[string]string{"ticket": token})
	var ready map[string]any
	if e = websocket.JSON.Receive(ws, &ready); e != nil || ready["ready"] != true {
		t.Fatalf("capture not ready: %v %v", ready, e)
	}
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		tick := time.NewTicker(20 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-stop:
				return
			case <-tick.C:
				if websocket.Message.Send(ws, make([]byte, 320)) != nil {
					return
				}
			}
		}
	}()
	received := 0
	frames := 0
	for received < 32000 {
		var b []byte
		if e = websocket.Message.Receive(ws, &b); e != nil {
			t.Fatal(e)
		}
		received += len(b)
		frames++
	}
	t.Logf("Actual USB capture: %d PCM bytes in %d frames; silent uplink delivered; no call/config commands", received, frames)
}
