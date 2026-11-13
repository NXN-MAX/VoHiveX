package main

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var errSerialBusy = errors.New("serial port busy")

type modemDiscovery struct {
	sysRoot string
	devRoot string
	probe   func(context.Context, string) (string, error)
	gate    chan struct{}
}

func newModemDiscovery(sysRoot, devRoot string) *modemDiscovery {
	return &modemDiscovery{sysRoot: sysRoot, devRoot: devRoot, probe: probeSerialIMEI, gate: make(chan struct{}, 1)}
}

func sysText(path string) string {
	raw, _ := os.ReadFile(path)
	return strings.TrimSpace(string(raw))
}

func sysDriver(path string) string {
	target, _ := os.Readlink(filepath.Join(path, "driver"))
	return filepath.Base(target)
}

var usbDeviceName = regexp.MustCompile(`^[0-9]+-[0-9]+(?:\.[0-9]+)*$`)
var usbTTYName = regexp.MustCompile(`^ttyUSB[0-9]+$`)

// Only this supported ECM layout is added to the upstream discovery result.
// The diagnostic and GNSS ports are deliberately never probed.
func (d *modemDiscovery) ecmCandidates() []map[string]any {
	paths, _ := filepath.Glob(filepath.Join(d.sysRoot, "bus", "usb", "devices", "*"))
	rows := []map[string]any{}
	for _, path := range paths {
		if !usbDeviceName.MatchString(filepath.Base(path)) || sysText(filepath.Join(path, "idVendor")) != "2ca3" || sysText(filepath.Join(path, "idProduct")) != "4006" {
			continue
		}
		if sysText(filepath.Join(path, "bConfigurationValue")) != "1" ||
			sysText(path+":1.2/bInterfaceClass") != "ff" ||
			sysText(path+":1.4/bInterfaceClass") != "02" ||
			sysText(path+":1.4/bInterfaceSubClass") != "06" ||
			sysText(path+":1.5/bInterfaceClass") != "0a" {
			continue
		}
		row := map[string]any{"usb_path": filepath.Base(path), "mode": "ecm", "driver_name": "cdc_ether",
			"vendor_id": 0x2ca3, "product_id": 0x4006, "configured": false, "control_path": "", "at_port": "", "imei": ""}
		ports := map[string]bool{}
		for _, pattern := range []string{path + ":1.2/ttyUSB*/tty/ttyUSB*", path + ":1.2/tty/ttyUSB*"} {
			matches, _ := filepath.Glob(pattern)
			for _, match := range matches {
				name := filepath.Base(match)
				if usbTTYName.MatchString(name) {
					ports[filepath.Join(d.devRoot, name)] = true
				}
			}
		}
		if len(ports) == 1 {
			for port := range ports {
				if _, err := os.Stat(port); err == nil {
					row["at_port"] = port
					row["at_ports"] = []string{port}
				}
			}
		}
		networks, _ := filepath.Glob(path + ":1.4/net/*")
		if len(networks) == 1 {
			row["net_interface"] = filepath.Base(networks[0])
			row["network_capable"] = true
		}
		switch {
		case row["at_port"] == "":
			degradeDiscovery(row, "missing_at_port", "USB 模组已连接，但 AT 串口尚未就绪，请检查 option 驱动和设备映射。")
		case len(networks) != 1:
			degradeDiscovery(row, "missing_network_interface", "ECM 网卡尚未就绪，请检查 cdc_ether 驱动。")
		case sysDriver(path+":1.2") != "option" || sysDriver(path+":1.4") != "cdc_ether" || sysDriver(path+":1.5") != "cdc_ether":
			degradeDiscovery(row, "driver_conflict", "USB 接口尚未绑定预期驱动，请检查容器驱动日志。")
		}
		row["discovery_key"] = row["usb_path"].(string) + "|" + row["at_port"].(string)
		rows = append(rows, row)
		if len(rows) == 16 {
			break
		}
	}
	return rows
}

func degradeDiscovery(row map[string]any, code, reason string) {
	row["degraded"], row["discovery_error"], row["degraded_reason"] = true, code, reason
}

func discoveryContains(rows []map[string]any, candidate map[string]any) bool {
	for _, known := range rows {
		if known["usb_path"] == candidate["usb_path"] || (candidate["at_port"] != "" && known["at_port"] == candidate["at_port"]) {
			return true
		}
	}
	return false
}

func configuredECM(row map[string]any, configured []map[string]any) map[string]any {
	for _, device := range configured {
		usb, _ := device["usb_path"].(string)
		port, _ := device["at_port"].(string)
		if (usb != "" && usb == row["usb_path"]) || (port != "" && port == row["at_port"]) {
			return device
		}
	}
	return nil
}

func configuredIMEI(imei string, configured []map[string]any) (map[string]any, bool) {
	var match map[string]any
	for _, device := range configured {
		identity, _ := device["modem_imei"].(string)
		if identity != imei {
			continue
		}
		if match != nil {
			return nil, true
		}
		match = device
	}
	return match, false
}

func markConfigured(row, device map[string]any, withIMEI bool) {
	row["configured"], row["configured_id"] = true, device["id"]
	if identity, _ := device["modem_imei"].(string); withIMEI && validModemIMEI(identity) {
		row["imei"] = identity
	}
}

// The core keeps USB/AT paths in live workers, while its saved YAML may contain
// only an ID and IMEI. Use live bindings to avoid probing a worker's owned port.
func (app *application) discoveryConfiguration(r *http.Request, runtime map[string]any) ([]map[string]any, error) {
	cfg, _, err := readCoreConfig(app.configPath)
	if err != nil {
		return nil, err
	}
	configured := make([]map[string]any, 0, len(cfg.Devices))
	for _, saved := range cfg.Devices {
		device := make(map[string]any, len(saved)+1)
		for key, value := range saved {
			device[key] = value
		}
		device["_runtime_running"] = false
		configured = append(configured, device)
	}
	for _, running := range deviceRows(runtime) {
		if running["running"] != true && running["healthy"] != true {
			continue
		}
		id, _ := running["id"].(string)
		if id == "" {
			return nil, errors.New("invalid active device identity")
		}
		status, value, err := app.upstreamJSON(r, http.MethodGet, "/api/devices/"+url.PathEscape(id)+"/config", nil, 2*time.Second)
		if err != nil || status != http.StatusOK {
			return nil, errors.New("active device configuration unavailable")
		}
		object, _ := value.(map[string]any)
		live, ok := object["config"].(map[string]any)
		if !ok || live["id"] != id {
			return nil, errors.New("invalid active device configuration")
		}
		usb, _ := live["usb_path"].(string)
		port, _ := live["at_port"].(string)
		if live["device_backend"] == "at" && usb == "" && port == "" {
			return nil, errors.New("active AT port ownership unavailable")
		}
		found := false
		for _, device := range configured {
			if device["id"] != id {
				continue
			}
			if device["modem_imei"] != live["modem_imei"] {
				return nil, errors.New("device configuration changed during discovery")
			}
			for _, key := range []string{"usb_path", "at_port"} {
				device[key] = live[key]
			}
			device["_runtime_running"] = true
			found = true
		}
		if !found {
			return nil, errors.New("device configuration changed during discovery")
		}
	}
	return configured, nil
}

func (d *modemDiscovery) probeIMEI(ctx context.Context, port string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	select {
	case d.gate <- struct{}{}:
		defer func() { <-d.gate }()
	case <-ctx.Done():
		return "", ctx.Err()
	}
	probeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return d.probe(probeCtx, port)
}

func (d *modemDiscovery) appendECM(ctx context.Context, existing []map[string]any, configured []map[string]any, configErr error, withIMEI bool) []map[string]any {
	rows := append([]map[string]any{}, existing...)
	for _, row := range d.ecmCandidates() {
		if discoveryContains(rows, row) {
			continue
		}
		device := configuredECM(row, configured)
		if configErr != nil {
			degradeDiscovery(row, "configuration_unavailable", "无法读取当前设备配置，暂不探测串口，请检查配置文件。")
		} else if device != nil && (device["_runtime_running"] == true || !withIMEI) {
			markConfigured(row, device, withIMEI)
		} else if withIMEI && row["degraded"] != true {
			imei, err := d.probeIMEI(ctx, row["at_port"].(string))
			switch {
			case errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded):
				degradeDiscovery(row, "discovery_timeout", "设备探测已超时，请重新扫描。")
			case errors.Is(err, errSerialBusy):
				degradeDiscovery(row, "serial_busy", "AT 串口被占用，请检查主机上的 ModemManager 或其他串口程序，释放后重试。")
			case err != nil || !validModemIMEI(imei):
				degradeDiscovery(row, "identity_unavailable", "无法读取模组 IMEI，请检查 AT 串口后重试。")
			default:
				row["imei"] = imei
				if saved, ambiguous := configuredIMEI(imei, configured); ambiguous {
					degradeDiscovery(row, "configuration_conflict", "多个设备配置绑定了同一模组，请检查设备配置。")
				} else if saved != nil {
					markConfigured(row, saved, true)
				}
			}
		}
		rows = append(rows, row)
	}
	return rows
}

func (app *application) discoveredDevicesAPI(w http.ResponseWriter, r *http.Request) {
	runtime, status, err := app.validateAuth(r)
	if err != nil {
		writeJSON(w, status, map[string]any{"error": err.Error()})
		return
	}
	path := "/api/devices/discovered"
	if r.URL.RawQuery != "" {
		path += "?" + r.URL.RawQuery
	}
	status, value, err := app.upstreamJSON(r, http.MethodGet, path, nil, 15*time.Second)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": "设备发现服务暂时不可用"})
		return
	}
	if status != http.StatusOK || app.discovery == nil {
		writeJSON(w, status, value)
		return
	}
	data, ok := value.(map[string]any)
	if !ok {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": "设备发现响应格式无效"})
		return
	}
	if list, present := data["devices"]; !present || (list != nil && !isDiscoveryList(list)) {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": "设备发现响应格式无效"})
		return
	}
	// New images discover this layout natively. Preserve that complete response
	// without extra config requests or serial access; retain the older-core fallback.
	existing := deviceRows(data)
	missing := false
	for _, candidate := range app.discovery.ecmCandidates() {
		if !discoveryContains(existing, candidate) {
			missing = true
			break
		}
	}
	if !missing {
		writeJSON(w, status, data)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	configured, configErr := app.discoveryConfiguration(r.Clone(ctx), runtime)
	data["devices"] = app.discovery.appendECM(ctx, existing, configured, configErr, r.URL.Query().Get("with_imei") == "1")
	writeJSON(w, status, data)
}

func isDiscoveryList(value any) bool {
	rows, ok := value.([]any)
	if !ok {
		return false
	}
	for _, row := range rows {
		if _, ok := row.(map[string]any); !ok {
			return false
		}
	}
	return true
}

func validModemIMEI(value string) bool {
	if len(value) != 15 {
		return false
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}
