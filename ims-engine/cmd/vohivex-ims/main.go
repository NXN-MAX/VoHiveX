package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

func main() {
	localUSB := flag.Bool("usb-local", false, "use macOS USB AT directly; requires RF off")
	config := flag.String("config", "/app/config/config.yaml", "core configuration")
	base := flag.String("core", "http://127.0.0.1:7576", "core loopback URL")
	device := flag.String("device", "", "device ID")
	socket := flag.String("socket", "/app/data/ims/control.sock", "private Unix socket")
	mcc := flag.String("mcc", "", "SIM home MCC")
	mnc := flag.String("mnc", "", "SIM home MNC")
	epdg := flag.String("epdg", "", "optional ePDG host")
	sipTransport := flag.String("sip-transport", "tcp", "SIP transport: tcp or udp")
	probe := flag.Bool("probe", false, "register only; exit after first real IMS result")
	dir := flag.String("recordings", "/app/data/ims/recordings", "private recording directory")
	flag.Parse()
	if *sipTransport != "tcp" && *sipTransport != "udp" {
		fmt.Fprintln(os.Stderr, "invalid SIP transport")
		os.Exit(2)
	}
	if *device == "" {
		fmt.Fprintln(os.Stderr, "device is required")
		os.Exit(2)
	}
	var c *coreClient
	var err error
	if *localUSB {
		c, err = newLocalCore(*device)
	} else {
		c, err = newCore(*base, *device, *config)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer c.Close()
	e := &engine{core: c, mcc: *mcc, mnc: *mnc, epdg: *epdg, recordDir: *dir, sipTransport: *sipTransport}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()
	if err = os.MkdirAll(filepath.Dir(*socket), 0700); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	// Refuse to replace an existing listener; stale sockets need explicit cleanup.
	ln, err := net.Listen("unix", *socket)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer os.Remove(*socket)
	os.Chmod(*socket, 0600)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /status", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, e.snapshot()) })
	mux.HandleFunc("POST /calls", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Number string `json:"number"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048)).Decode(&in); err != nil {
			writeJSON(w, 400, map[string]string{"error": "invalid request"})
			return
		}
		c, err := e.dial(in.Number)
		if err != nil {
			writeJSON(w, 409, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 200, c)
	})
	mux.HandleFunc("POST /calls/{id}/hangup", func(w http.ResponseWriter, r *http.Request) {
		if err := e.hangup(r.PathValue("id")); err != nil {
			writeJSON(w, 409, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	})
	mux.HandleFunc("POST /calls/{id}/recording/{action}", func(w http.ResponseWriter, r *http.Request) {
		e.mu.Lock()
		m := e.media[r.PathValue("id")]
		e.mu.Unlock()
		if m == nil {
			writeJSON(w, 404, map[string]string{"error": "call not found"})
			return
		}
		var err error
		switch r.PathValue("action") {
		case "start":
			err = m.startRecording()
		case "stop":
			_, err = m.stopRecording()
		default:
			err = errors.New("unknown action")
		}
		if err != nil {
			writeJSON(w, 409, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	})
	server := http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go server.Serve(ln)
	defer server.Close()
	done := make(chan error, 1)
	go func() {
		err := e.run(ctx)
		if err != nil && ctx.Err() == nil {
			e.stage("error", err)
		}
		done <- err
	}()
	timer := time.NewTicker(time.Second)
	defer timer.Stop()
	for {
		select {
		case err := <-done:
			if err != nil && ctx.Err() == nil {
				writeJSON(os.Stdout, 0, e.snapshot())
				server.Close()
				os.Remove(*socket)
				os.Exit(1)
			}
			return
		case <-ctx.Done():
			select {
			case <-done:
			case <-time.After(10 * time.Second):
			}
			return
		case <-timer.C:
			if *probe && e.snapshot().Registered {
				writeJSON(os.Stdout, 0, e.snapshot())
				cancel()
				select {
				case <-done:
				case <-time.After(5 * time.Second):
				}
				return
			}
		}
	}
}
func writeJSON(w interface{ Write([]byte) (int, error) }, status int, v any) {
	if rw, ok := w.(http.ResponseWriter); ok {
		rw.Header().Set("Content-Type", "application/json")
		rw.WriteHeader(status)
	}
	_ = json.NewEncoder(w).Encode(v)
}
