package main

import (
	"context"
	"errors"
	"net/http"
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

func (d *modemDiscovery) appendECM(ctx context.Context, existing []map[string]any, configured []map[string]any, configErr error, withIMEI bool) []map[string]any {
	rows := append([]map[string]any{}, existing...)
	for _, row := range d.ecmCandidates() {
		duplicate := false
		for _, known := range rows {
			if known["usb_path"] == row["usb_path"] || (row["at_port"] != "" && known["at_port"] == row["at_port"]) {
				duplicate = true
				break
			}
		}
		if duplicate {
			continue
		}
		if device := configuredECM(row, configured); device != nil {
			row["configured"], row["configured_id"] = true, device["id"]
			if withIMEI {
				row["imei"] = device["modem_imei"]
			}
		} else if configErr != nil {
			degradeDiscovery(row, "configuration_unavailable", "无法读取当前设备配置，暂不探测串口，请检查配置文件。")
		} else if withIMEI && row["degraded"] != true {
			select {
			case d.gate <- struct{}{}:
				probeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
				imei, err := d.probe(probeCtx, row["at_port"].(string))
				cancel()
				<-d.gate
				switch {
				case errors.Is(err, errSerialBusy):
					degradeDiscovery(row, "serial_busy", "AT 串口被占用，请检查主机上的 ModemManager 或其他串口程序，释放后重试。")
				case err != nil || !validModemIMEI(imei):
					degradeDiscovery(row, "identity_unavailable", "无法读取模组 IMEI，请检查 AT 串口后重试。")
				default:
					row["imei"] = imei
				}
			case <-ctx.Done():
				degradeDiscovery(row, "discovery_timeout", "设备探测已超时，请重新扫描。")
			}
		}
		rows = append(rows, row)
	}
	return rows
}

func (app *application) discoveredDevicesAPI(w http.ResponseWriter, r *http.Request) {
	if _, status, err := app.validateAuth(r); err != nil {
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
	var configured []map[string]any
	cfg, _, configErr := readCoreConfig(app.configPath)
	if configErr == nil {
		configured = cfg.Devices
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	data["devices"] = app.discovery.appendECM(ctx, deviceRows(data), configured, configErr, r.URL.Query().Get("with_imei") == "1")
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
