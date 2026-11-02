package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/websocket"
)

const nativeCallDevice = "local-usb-modem"

type mediaTicket struct {
	device  string
	expires time.Time
}
type browserMedia struct {
	helper    string
	mu        sync.Mutex
	tickets   map[string]mediaTicket
	active    *websocket.Conn
	streaming bool
	busy      bool
	cache     map[string]any
	cached    time.Time
	recMu     sync.Mutex
	recording *browserRecording
	uplink    []byte
}
type browserRecording struct {
	id      string
	call    callHistoryItem
	file    *os.File
	path    string
	frames  int64
	started time.Time
	err     error
}

func newBrowserMedia(helper string) *browserMedia {
	if helper == "" {
		return nil
	}
	return &browserMedia{helper: helper, tickets: map[string]mediaTicket{}}
}
func (m *browserMedia) run(ctx context.Context, op string, args ...string) (map[string]any, error) {
	cmd := exec.CommandContext(ctx, m.helper, append([]string{op}, args...)...)
	if op == "prepare" || op == "restore" {
		// Let the helper finish its rollback rather than killing it between USB writes.
		cmd.Cancel = func() error { return cmd.Process.Signal(os.Interrupt) }
		cmd.WaitDelay = 90 * time.Second
	}
	var out bytes.Buffer
	cmd.Stdout = &out
	// Module diagnostics stay out of API errors and never include credentials.
	err := cmd.Run()
	var value map[string]any
	_ = json.Unmarshal(out.Bytes(), &value)
	if detail, ok := value["error"].(string); ok {
		return nil, errors.New(detail)
	}
	if err != nil {
		return nil, errors.New("本机电话桥接失败，请检查模组连接与音频运行库")
	}
	return value, nil
}
func (m *browserMedia) status() map[string]any {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.busy {
		return map[string]any{"connected": true, "ready": false, "preparing": true}
	}
	if time.Since(m.cached) < 2*time.Second && m.cache != nil {
		return m.cache
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	s, e := m.run(ctx, "status")
	if e != nil {
		s = map[string]any{"connected": false, "ready": false, "detail": e.Error()}
	}
	m.cache = s
	m.cached = time.Now()
	return s
}
func (app *application) callDevices(snapshot map[string]any) []map[string]any {
	rows := deviceRows(snapshot)
	if app.media == nil {
		return rows
	}
	if app.localCallsOnly {
		rows = nil
	}
	s := app.media.status()
	// This is a separate, explicitly configured local modem. Never overwrite a
	// core device's VoWiFi policy or silently route its calls through this USB port.
	return append(rows, map[string]any{"id": nativeCallDevice, "name": "QDC507 · 本机 USB", "running": s["connected"] == true, "healthy": s["connected"] == true, "vowifi_enabled": false, "audio_preparing": s["preparing"] == true, "local_phone": ""})
}
func (app *application) browserAudioState(device string) map[string]any {
	state := app.calls.recordingState()
	if app.media == nil || device != nativeCallDevice {
		return state
	}
	m := app.media
	s := m.status()
	m.mu.Lock()
	linked := m.streaming
	m.mu.Unlock()
	detail := "请先准备模组音频，再连接网页音频"
	if s["ready"] == true {
		detail = "音频已准备，请连接网页音频"
	}
	if linked {
		detail = "网页双向音频已连接"
	}
	if s["preparing"] == true {
		detail = "正在准备模组音频"
	}
	if d, ok := s["detail"].(string); ok {
		detail = d
	}
	state = map[string]any{"available": linked, "active": false, "source": "browser", "browser_supported": true, "ready": s["ready"] == true, "connected": linked, "preparing": s["preparing"] == true, "restore_required": s["restore_required"] == true, "detail": detail}
	m.recMu.Lock()
	defer m.recMu.Unlock()
	if r := m.recording; r != nil {
		state["active"] = true
		state["call_id"] = r.call.ID
		state["id"] = r.id
		state["started_at"] = r.started.Unix()
	}
	return state
}
func (app *application) browserAudioAPI(w http.ResponseWriter, r *http.Request, devices []map[string]any) {
	if app.media == nil {
		writeJSON(w, 503, map[string]any{"error": "当前未配置本机电话音频桥接"})
		return
	}
	m := app.media
	var body struct {
		DeviceID string `json:"device_id"`
	}
	if !decodeJSON(w, r, &body, 4096) {
		return
	}
	device, found := callDevice(devices, body.DeviceID)
	if !found || body.DeviceID != nativeCallDevice || deviceCallTransport(device) != "cellular" {
		writeJSON(w, 409, map[string]any{"error": "此音频接口仅用于所选本机蜂窝模组，不会切换 VoWiFi 模式"})
		return
	}
	path := r.URL.Path
	if path == "/api/calls/audio/ticket" {
		s := m.status()
		m.mu.Lock()
		defer m.mu.Unlock()
		if m.busy || m.active != nil || s["ready"] != true {
			writeJSON(w, 409, map[string]any{"error": "音频尚未准备好或已有网页占用"})
			return
		}
		for k, t := range m.tickets {
			if time.Now().After(t.expires) {
				delete(m.tickets, k)
			}
		}
		if len(m.tickets) >= 8 {
			writeJSON(w, 429, map[string]any{"error": "音频连接请求过多"})
			return
		}
		b := make([]byte, 32)
		if _, e := rand.Read(b); e != nil {
			writeJSON(w, 500, map[string]any{"error": "无法创建音频连接"})
			return
		}
		ticket := hex.EncodeToString(b)
		m.tickets[ticket] = mediaTicket{body.DeviceID, time.Now().Add(20 * time.Second)}
		writeJSON(w, 201, map[string]any{"ticket": ticket, "sample_rate": 8000})
		return
	}
	if path != "/api/calls/audio/prepare" && path != "/api/calls/audio/restore" {
		writeJSON(w, 404, map[string]any{"error": "音频操作不存在"})
		return
	}
	m.mu.Lock()
	if m.busy || m.active != nil {
		m.mu.Unlock()
		writeJSON(w, 409, map[string]any{"error": "请先断开网页音频，再操作模组接口"})
		return
	}
	m.busy = true
	m.mu.Unlock()
	defer func() { m.mu.Lock(); m.busy = false; m.cached = time.Time{}; m.mu.Unlock() }()
	lock := app.calls.deviceLock(body.DeviceID)
	lock.Lock()
	defer lock.Unlock()
	// Complete a prepared hardware transaction even if the browser navigates
	// away. The helper owns its rollback snapshot; callers can retry restore.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	op := "prepare"
	if strings.HasSuffix(path, "restore") {
		op = "restore"
	}
	result, e := m.run(ctx, op)
	if e != nil {
		writeJSON(w, 502, map[string]any{"error": e.Error()})
		return
	}
	writeJSON(w, 200, result)
}
func (m *browserMedia) consumeTicket(ticket string) bool {
	t, ok := m.tickets[ticket]
	delete(m.tickets, ticket)
	return ok && t.device == nativeCallDevice && time.Now().Before(t.expires)
}
func (app *application) browserAudioStream(w http.ResponseWriter, r *http.Request) {
	if app.media == nil || r.Header.Get("Origin") == "" || !sameOrigin(r) {
		http.Error(w, "forbidden", 403)
		return
	}
	websocket.Server{Handshake: func(c *websocket.Config, r *http.Request) error {
		if r.Header.Get("Origin") == "" || !sameOrigin(r) {
			return errors.New("cross-origin audio denied")
		}
		return nil
	}, Handler: func(ws *websocket.Conn) { app.serveBrowserAudio(ws) }}.ServeHTTP(w, r)
}
func (app *application) serveBrowserAudio(ws *websocket.Conn) {
	m := app.media
	defer ws.Close()
	ws.MaxPayloadBytes = 4096
	_ = ws.SetDeadline(time.Now().Add(5 * time.Second))
	var auth struct {
		Ticket string `json:"ticket"`
	}
	if websocket.JSON.Receive(ws, &auth) != nil {
		return
	}
	m.mu.Lock()
	if m.busy || m.active != nil || !m.consumeTicket(auth.Ticket) {
		m.mu.Unlock()
		return
	}
	m.active = ws
	m.mu.Unlock()
	defer func() {
		_, _ = m.stopRecording(app.calls)
		m.mu.Lock()
		m.active = nil
		m.streaming = false
		m.cached = time.Time{}
		m.mu.Unlock()
	}()
	cmd := exec.Command(m.helper, "stream")
	in, e := cmd.StdinPipe()
	if e != nil {
		return
	}
	out, e := cmd.StdoutPipe()
	if e != nil {
		return
	}
	if e = cmd.Start(); e != nil {
		return
	}
	defer func() {
		in.Close()
		out.Close()
		_ = cmd.Process.Signal(os.Interrupt)
		done := make(chan struct{})
		go func() { _ = cmd.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			_ = cmd.Process.Kill()
			<-done
		}
	}()
	// Confirm actual CoreAudio samples before claiming a working connection.
	frames := make(chan []byte, 1)
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		defer close(frames)
		for {
			h := make([]byte, 4)
			if _, e := io.ReadFull(out, h); e != nil {
				return
			}
			n := int(binary.LittleEndian.Uint32(h))
			if n < 2 || n > 3200 || n%2 != 0 {
				return
			}
			b := make([]byte, n)
			if _, e := io.ReadFull(out, b); e != nil {
				return
			}
			select {
			case frames <- b:
			case <-stop:
				return
			}
		}
	}()
	var first []byte
	select {
	case first = <-frames:
		if len(first) == 0 {
			return
		}
	case <-time.After(6 * time.Second):
		return
	}
	_ = ws.SetDeadline(time.Time{})
	if websocket.JSON.Send(ws, map[string]any{"ready": true, "sample_rate": 8000}) != nil {
		return
	}
	m.mu.Lock()
	m.streaming = true
	m.mu.Unlock()
	go func() {
		defer ws.Close()
		defer in.Close()
		defer out.Close()
		for {
			_ = ws.SetReadDeadline(time.Now().Add(20 * time.Second))
			var b []byte
			if websocket.Message.Receive(ws, &b) != nil {
				return
			}
			if len(b) < 2 || len(b) > 3200 || len(b)%2 != 0 {
				return
			}
			frame := make([]byte, 4+len(b))
			binary.LittleEndian.PutUint32(frame, uint32(len(b)))
			copy(frame[4:], b)
			if _, e := in.Write(frame); e != nil {
				return
			}
			m.recMu.Lock()
			if m.recording != nil {
				m.uplink = append(m.uplink, b...)
				if len(m.uplink) > 16000 {
					m.uplink = m.uplink[len(m.uplink)-16000:]
				}
			}
			m.recMu.Unlock()
		}
	}()
	for b := first; ; {
		m.recordDownlink(b)
		_ = ws.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if websocket.Message.Send(ws, b) != nil {
			return
		}
		select {
		case b = <-frames:
			if len(b) == 0 {
				return
			}
		case <-time.After(5 * time.Second):
			return
		}
	}
}

func (m *browserMedia) startRecording(call callHistoryItem, dir string) (map[string]any, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	linked := m.streaming
	if !linked {
		return nil, invalidError{"请先连接网页音频"}
	}
	m.recMu.Lock()
	defer m.recMu.Unlock()
	if m.recording != nil {
		return nil, invalidError{"已有通话正在录音"}
	}
	b := make([]byte, 16)
	if _, e := rand.Read(b); e != nil {
		return nil, e
	}
	id := hex.EncodeToString(b)
	p := filepath.Join(dir, "."+id+".part.wav")
	f, e := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	if _, e = f.Write(make([]byte, 44)); e != nil {
		f.Close()
		os.Remove(p)
		return nil, e
	}
	m.recording = &browserRecording{id: id, call: call, file: f, path: p, started: time.Now()}
	m.uplink = nil
	return map[string]any{"active": true, "id": id, "call_id": call.ID}, nil
}
func (m *browserMedia) recordDownlink(down []byte) {
	m.recMu.Lock()
	defer m.recMu.Unlock()
	r := m.recording
	if r == nil {
		return
	}
	if r.err != nil {
		return
	}
	if r.frames*4+int64(len(down)*2) > (1<<32)-44 {
		r.err = errors.New("录音超过 WAV 文件大小上限")
		return
	}
	stereo := make([]byte, len(down)*2)
	for i := 0; i < len(down); i += 2 {
		copy(stereo[i*2:], down[i:i+2])
		if i+2 <= len(m.uplink) {
			copy(stereo[i*2+2:], m.uplink[i:i+2])
		}
	}
	take := len(down)
	if take > len(m.uplink) {
		take = len(m.uplink)
	}
	m.uplink = m.uplink[take:]
	if n, e := r.file.Write(stereo); e == nil && n == len(stereo) {
		r.frames += int64(len(down) / 2)
	} else {
		r.err = errors.New("录音写入失败，请检查磁盘空间")
	}
}
func (m *browserMedia) stopRecording(manager *callManager) (callRecording, error) {
	return m.stopRecordingID(manager, "")
}
func (m *browserMedia) stopRecordingID(manager *callManager, expected string) (callRecording, error) {
	m.recMu.Lock()
	defer m.recMu.Unlock()
	r := m.recording
	if expected != "" && (r == nil || r.id != expected) {
		return callRecording{}, nil
	}
	m.recording = nil
	m.uplink = nil
	if r == nil {
		return callRecording{}, invalidError{"当前没有正在录制的网页通话"}
	}
	var item callRecording
	if r.frames == 0 || r.err != nil {
		r.file.Close()
		os.Remove(r.path)
		if r.err != nil {
			return item, r.err
		}
		return item, errors.New("未收到录音音频")
	}
	h := make([]byte, 44)
	copy(h, "RIFF")
	binary.LittleEndian.PutUint32(h[4:], uint32(36+r.frames*4))
	copy(h[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(h[16:], 16)
	binary.LittleEndian.PutUint16(h[20:], 1)
	binary.LittleEndian.PutUint16(h[22:], 2)
	binary.LittleEndian.PutUint32(h[24:], 8000)
	binary.LittleEndian.PutUint32(h[28:], 32000)
	binary.LittleEndian.PutUint16(h[32:], 4)
	binary.LittleEndian.PutUint16(h[34:], 16)
	copy(h[36:], "data")
	binary.LittleEndian.PutUint32(h[40:], uint32(r.frames*4))
	_, e := r.file.WriteAt(h, 0)
	if e == nil {
		e = r.file.Sync()
	}
	r.file.Close()
	if e != nil {
		os.Remove(r.path)
		return item, e
	}
	final := filepath.Join(manager.recordingsDir, r.id+".wav")
	if e = os.Rename(r.path, final); e != nil {
		return item, e
	}
	item = callRecording{ID: r.id, CallID: r.call.ID, DeviceID: r.call.DeviceID, Number: r.call.Number, Direction: r.call.Direction, StartedAt: r.started.Unix(), EndedAt: time.Now().Unix(), DurationSeconds: r.frames / 8000, MIMEType: "audio/wav", Size: 44 + r.frames*4, AudioPath: "/api/calls/recordings/" + r.id + "/audio", fileName: r.id + ".wav"}
	tx, e := manager.db.Begin()
	if e != nil {
		os.Remove(final)
		return item, e
	}
	defer tx.Rollback()
	_, e = tx.Exec(`INSERT INTO call_recordings(id,call_id,device_id,number,direction,started_at,ended_at,duration_seconds,file_name,mime_type,size)VALUES(?,?,?,?,?,?,?,?,?,?,?)`, item.ID, item.CallID, item.DeviceID, item.Number, item.Direction, item.StartedAt, item.EndedAt, item.DurationSeconds, item.fileName, item.MIMEType, item.Size)
	if e == nil {
		_, e = tx.Exec(`UPDATE call_history SET recording_id=? WHERE id=?`, item.ID, item.CallID)
	}
	if e == nil {
		e = tx.Commit()
	}
	if e != nil {
		os.Remove(final)
	}
	return item, e
}
func (app *application) finishBrowserRecording() {
	if app.media == nil {
		return
	}
	app.media.recMu.Lock()
	r := app.media.recording
	app.media.recMu.Unlock()
	if r == nil {
		return
	}
	call, e := app.calls.callByID(r.call.ID)
	if e == nil && call.EndedAt != nil {
		_, _ = app.media.stopRecordingID(app.calls, r.id)
	}
}

func (m *browserMedia) Close(manager *callManager) {
	if m == nil {
		return
	}
	m.mu.Lock()
	if m.active != nil {
		_ = m.active.Close()
	}
	m.mu.Unlock()
	_, _ = m.stopRecording(manager)
}
