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
	"sync"
	"syscall"
	"time"
)

const version = "2.1.5"

type application struct {
	configPath     string
	assets         string
	upstream       string
	smsLogPath     string
	appLogPath     string
	logMu          sync.Mutex
	proxy          *proxyManager
	agreement      *agreementStore
	webhook        *webhookAdapter
	tasks          *taskStore
	archive        *smsArchive
	calls          *callManager
	media          *browserMedia
	localCallsOnly bool
	network        *networkDebugManager
	discovery      *modemDiscovery
	smsMu          sync.Mutex
	coreAuthMu     sync.Mutex
	coreToken      string
	coreExpiry     time.Time
	coreRetry      time.Time
	qmiSendSMS     func(context.Context, qmiSMSDevice, string, string) (int, error)
	server         *http.Server
	started        time.Time
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
	probeQMIPath := flag.String("probe-qmi-wms", "", "open a QMI WMS service through qmi-proxy, print read-only diagnostics, and exit")
	probeVoicePath := flag.String("probe-qmi-voice", "", "query QMI VOICE capabilities and call states through qmi-proxy without dialing, and exit")
	probeProxyPath := flag.String("probe-qmi-proxy-path", "", "qmi-proxy socket used by QMI diagnostic probes")
	probeProxyExecutable := flag.String("probe-qmi-proxy-executable", "", "qmi-proxy executable used by QMI diagnostic probes")
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return nil
	}
	if *probeVoicePath != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		result, err := probeQMIVoice(ctx, qmiSMSDevice{ControlPath: *probeVoicePath, UseProxy: true, ProxyPath: *probeProxyPath, ProxyExecutable: *probeProxyExecutable})
		if err != nil {
			return err
		}
		fmt.Println(result)
		return nil
	}
	if *probeQMIPath != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		result, err := probeQMIWMS(ctx, qmiSMSDevice{ControlPath: *probeQMIPath, UseProxy: true, ProxyPath: *probeProxyPath, ProxyExecutable: *probeProxyExecutable})
		if err != nil {
			return err
		}
		fmt.Println(result)
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
	agreement, err := newAgreementStore(filepath.Join(dataDir, "account-agreement.json"))
	if err != nil {
		return fmt.Errorf("load account agreement: %w", err)
	}
	webhook, err := newWebhookAdapter(filepath.Join(dataDir, "webhook-adapter.json"), fmt.Sprintf("http://127.0.0.1:%d%s", port, webhookRelayPath))
	if err != nil {
		return fmt.Errorf("load webhook adapter: %w", err)
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
	calls, err := newCallManager(
		getenv("CALLS_DB", filepath.Join(dataDir, "calls.sqlite3")),
		getenv("CALL_RECORDINGS_DIR", filepath.Join(dataDir, "call-recordings")),
	)
	if err != nil {
		archive.Close()
		return err
	}
	app := &application{
		configPath:     configPath,
		assets:         getenv("SCHEDULER_ASSETS", "/opt/vohivex/scheduler/assets"),
		upstream:       getenv("VOHIVE_UPSTREAM_HOST", "127.0.0.1") + ":" + getenv("VOHIVE_UPSTREAM_PORT", "7576"),
		smsLogPath:     getenv("VOHIVEX_SMS_LOG", "/app/logs/sms-send.log"),
		appLogPath:     getenv("VOHIVEX_APP_LOG", "/app/logs/app.log"),
		proxy:          proxy,
		agreement:      agreement,
		webhook:        webhook,
		tasks:          tasks,
		archive:        archive,
		calls:          calls,
		media:          newBrowserMedia(os.Getenv("VOHIVEX_CALL_BRIDGE")),
		localCallsOnly: os.Getenv("VOHIVEX_LOCAL_CALLS_ONLY") == "1",
		network:        newNetworkDebugManager(),
		discovery:      newModemDiscovery(getenv("QMI_SYS_ROOT", "/host-sys"), "/dev"),
		qmiSendSMS:     sendQmiSMS,
		started:        time.Now(),
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	defer tasks.Close()
	defer archive.Close()
	defer calls.Close()
	defer app.media.Close(calls)
	defer app.network.Close()
	defer proxy.Close()

	go app.runWorkers(ctx)
	go runNetworkDebugProxy(ctx)
	app.server = &http.Server{
		Addr:              fmt.Sprintf("%s:%d", getenv("VOHIVEX_LISTEN_HOST", "0.0.0.0"), port),
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
