package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
)

const version = "2.1.2"

type application struct {
	configPath string
	assets     string
	upstream   string
	proxy      *proxyManager
	tasks      *taskStore
	archive    *smsArchive
	server     *http.Server
	started    time.Time
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func run() error {
	syscall.Umask(0o077)
	prepare := flag.Bool("prepare-config", false, "create or migrate the VoHiveX core configuration and exit")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return nil
	}
	configPath := getenv("CONFIG_PATH", "/app/config/config.yaml")
	if *prepare {
		return prepareConfig(configPath)
	}
	port, err := strconv.Atoi(getenv("SCHEDULER_PORT", "7575"))
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("invalid SCHEDULER_PORT")
	}
	dataDir := getenv("VOHIVEX_DATA", "/app/data")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return err
	}
	proxy, err := newProxyManager(filepath.Join(dataDir, "managed-proxy"), getenv("MIHOMO_BINARY", "/opt/vohivex/proxy/mihomo"))
	if err != nil {
		return err
	}
	tasks, err := newTaskStore(getenv("SCHEDULER_DB", filepath.Join(dataDir, "scheduled-sms.sqlite3")))
	if err != nil {
		return err
	}
	archive, err := newSMSArchive(getenv("SMS_ARCHIVE_DB", filepath.Join(dataDir, "imported-sms.sqlite3")))
	if err != nil {
		return err
	}
	app := &application{
		configPath: configPath,
		assets:     getenv("SCHEDULER_ASSETS", "/opt/vohivex/scheduler/assets"),
		upstream:   getenv("VOHIVE_UPSTREAM_HOST", "127.0.0.1") + ":" + getenv("VOHIVE_UPSTREAM_PORT", "7576"),
		proxy:      proxy,
		tasks:      tasks,
		archive:    archive,
		started:    time.Now(),
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	defer tasks.Close()
	defer archive.Close()
	defer proxy.Close()

	go app.runWorkers(ctx)
	app.server = &http.Server{
		Addr:              fmt.Sprintf("0.0.0.0:%d", port),
		Handler:           app.routes(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       180 * time.Second,
		WriteTimeout:      180 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    64 << 10,
	}
	errCh := make(chan error, 1)
	go func() {
		log.Printf("VoHiveX Go gateway %s listening on %s", version, app.server.Addr)
		errCh <- app.server.ListenAndServe()
	}()
	select {
	case <-ctx.Done():
		shutdown, stop := context.WithTimeout(context.Background(), 15*time.Second)
		defer stop()
		return app.server.Shutdown(shutdown)
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func main() {
	os.Exit(func() int {
		if err := run(); err != nil {
			log.Printf("VoHiveX gateway failed: %v", err)
			return 1
		}
		return 0
	}())
}
