//go:build darwin && cgo

package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const runtimePath = "/tmp/vohivex-call-audio"

func runtimeLocal() string { return os.Getenv("VOHIVEX_CALL_RUNTIME_DIR") }

var runtimeFiles = []struct {
	name, hash string
	size       int
	mode       uint32
}{
	{"qdc507_aprv3.ko", "3d82d3dec4f1e323201bba87156df9d41438e08314097353f2607f9117211d4a", 36664, 0644},
	{"qdc507_voice.ko", "ed3821682d5309969a01c764192c83feff9669c61ef237c69475cd1619cf296c", 999236, 0644},
	{"mavo-pcm-bridge.armv7", "88d47c15e61d1428a59c821fed804c2e6490e82859a085062f21966b58d167fc", 17860, 0755},
}

func verifyRuntimeFiles() error {
	for _, f := range runtimeFiles {
		b, e := os.ReadFile(filepath.Join(runtimeLocal(), f.name))
		if e != nil {
			return fmt.Errorf("缺少已校验的模组音频运行库: %s", f.name)
		}
		if len(b) != f.size || fmt.Sprintf("%x", sha256.Sum256(b)) != f.hash {
			return fmt.Errorf("音频运行库校验失败: %s", f.name)
		}
	}
	return nil
}

const runtimeDevices = "test -c /dev/snd/controlC0 && test -c /dev/snd/pcmC0D4p && test -c /dev/snd/pcmC0D4c && test -c /dev/snd/pcmC0D5p && test -c /dev/snd/pcmC0D6c && grep -Fq 'mdm9607-tomtom-i2s-snd-card' /proc/asound/cards"

func runtimeShell(a *adbClient, c string, timeout time.Duration) error {
	out, status, err := a.shellChecked(c, timeout)
	fmt.Fprintf(os.Stderr, "[%s] status=%d error=%v\n", c, status, err)
	for _, line := range strings.Split(strings.ReplaceAll(out, "\r", ""), "\n") {
		if !strings.HasPrefix(line, "__MAVO_STATUS_") {
			fmt.Fprintln(os.Stderr, line)
		}
	}
	if err != nil {
		return err
	}
	if status != 0 {
		return fmt.Errorf("remote command failed with exit %d", status)
	}
	return nil
}
func runtimeStep(step string) error {
	if step != "runtime-preflight" && step != "runtime-diagnostics" && step != "runtime-status" {
		if err := runtimeCheckIdle(); err != nil {
			return err
		}
	}
	a, err := openDJIUSBADB(modemLocation())
	if err != nil {
		return err
	}
	defer a.Close()
	if err := runtimeShell(a, `test "$(uname -r)" = 3.18.44 && test "$(id -u)" = 0`, 8*time.Second); err != nil {
		return err
	}
	switch step {
	case "runtime-preflight":
		for _, c := range []string{"readlink -f /tmp", "cat /proc/mounts", "cat /proc/modules", "ls -l /usr/bin/alsaucm_test /usr/bin/qti_audio_server_daemon /lib/ld-linux.so.3", "ls -l /run/voc_svr /run/alsaucm_test", "ps | grep -E 'alsaucm|audio_server|pcm-bridge'", "ls -l /dev/snd", "cat /sys/class/android_usb/f_audio/audio_enable", "ls -l /usr/share/alsa /etc/alsa"} {
			_ = runtimeShell(a, c, 8*time.Second)
		}
		return nil
	case "runtime-stage":
		// /tmp must resolve below the stock tmpfs, never persistent flash.
		guard := `test "$(readlink -f /tmp)" = /var/volatile/tmp && grep -q ' /var/volatile tmpfs ' /proc/mounts && test -z "$(cat /proc/modules)" && test ! -e '` + runtimePath + `'`
		if err := runtimeShell(a, guard, 8*time.Second); err != nil {
			return err
		}
		// Verify every local input before creating anything on the modem.
		blobs := make([][]byte, len(runtimeFiles))
		for i, f := range runtimeFiles {
			b, e := os.ReadFile(filepath.Join(runtimeLocal(), f.name))
			if e != nil {
				return e
			}
			if len(b) != f.size || fmt.Sprintf("%x", sha256.Sum256(b)) != f.hash {
				return fmt.Errorf("artifact mismatch: %s", f.name)
			}
			blobs[i] = b
		}
		if err := runtimeShell(a, "mkdir -m 700 '"+runtimePath+"'", 8*time.Second); err != nil {
			return err
		}
		for i, f := range runtimeFiles {
			if err := a.push(blobs[i], runtimePath+"/"+f.name, 0100000|f.mode, 30*time.Second); err != nil {
				return err
			}
			if err := runtimeShell(a, "test \"$(sha256sum '"+runtimePath+"/"+f.name+"' | cut -d ' ' -f 1)\" = "+f.hash, 8*time.Second); err != nil {
				return err
			}
			fmt.Fprintln(os.Stderr, "verified remote", f.name)
		}
		return nil
	case "runtime-load-apr", "runtime-load-voice":
		f := runtimeFiles[0]
		if step == "runtime-load-voice" {
			f = runtimeFiles[1]
			if err := runtimeShell(a, "grep -q '^qdc507_aprv3 ' /proc/modules && ! grep -q '^qdc507_voice ' /proc/modules", 8*time.Second); err != nil {
				return err
			}
		} else {
			if err := runtimeShell(a, `test -z "$(cat /proc/modules)"`, 8*time.Second); err != nil {
				return err
			}
		}
		if err := runtimeShell(a, "test \"$(sha256sum '"+runtimePath+"/"+f.name+"' | cut -d ' ' -f 1)\" = "+f.hash, 8*time.Second); err != nil {
			return err
		}
		err = runtimeShell(a, "insmod '"+runtimePath+"/"+f.name+"'", 20*time.Second)
		_ = runtimeShell(a, "cat /proc/modules; cat /proc/asound/cards; ls -l /dev/snd; dmesg | tail -n 60", 8*time.Second)
		return err
	case "runtime-diagnostics":
		return runtimeShell(a, "cat /proc/modules; cat /proc/asound/cards; cat /proc/asound/pcm; ls -l /dev/snd; ls -l /sys/class/sound; for p in /sys/class/sound/*/dev; do echo \"$p\"; cat \"$p\"; done; cat /proc/sys/kernel/hotplug; ps | grep -E 'udev|mdev'; dmesg | tail -n 30", 8*time.Second)
	case "runtime-calibrate":
		if err := runtimeShell(a, runtimeDevices+" && test -x /usr/bin/alsaucm_test && test -p /run/voc_svr && test -c /dev/ttyGS0", 8*time.Second); err != nil {
			return err
		}
		c := `test ! -e /run/alsaucm_test && test ! -e '` + runtimePath + `/calibration.pid' || exit 70; ` +
			`for p in /proc/[0-9]*/cmdline; do head=$(tr '\000' '\n' < "$p" 2>/dev/null | sed -n '1p'); test "$head" != /usr/bin/alsaucm_test || exit 71; done; ` +
			`nohup /usr/bin/alsaucm_test </dev/null >'` + runtimePath + `/calibration.log' 2>&1 & pid=$!; ` +
			`start=$(cut -d ' ' -f 22 "/proc/$pid/stat"); printf '%s %s\n' "$pid" "$start" >'` + runtimePath + `/calibration.pid'; ` +
			`n=0; while test "$n" -lt 50 && test ! -p /run/alsaucm_test; do kill -0 "$pid" || exit 72; sleep 0.1; n=$((n+1)); done; test -p /run/alsaucm_test || exit 73; ` +
			`printf 'open snd_soc_msm_9x07_Tomtom_I2S\n' > /run/alsaucm_test; printf 'set _verb VoLTE\n' > /run/alsaucm_test; printf 'set _enadev Auxpcm Rx\n' > /run/alsaucm_test; printf 'set _enadev Auxpcm Tx\n' > /run/alsaucm_test; ` +
			`n=0; while test "$n" -lt 100; do grep -q 'ACDB -> Sent VocProc Cal!' '` + runtimePath + `/calibration.log' && break; sleep 0.1; n=$((n+1)); done; grep -q 'ACDB -> Sent VocProc Cal!' '` + runtimePath + `/calibration.log'`
		err = runtimeShell(a, c, 25*time.Second)
		_ = runtimeShell(a, "tail -n 90 '"+runtimePath+"/calibration.log'", 8*time.Second)
		return err
	case "runtime-helper-check":
		if err := runtimeShell(a, runtimeDevices, 8*time.Second); err != nil {
			return err
		}
		return runtimeShell(a, "'"+runtimePath+"/mavo-pcm-bridge.armv7' --check", 15*time.Second)
	case "runtime-route":
		if err := runtimeShell(a, runtimeDevices+" && grep -q 'ACDB -> Sent VocProc Cal!' '"+runtimePath+"/calibration.log' && test ! -e '"+runtimePath+"/route.pid'", 8*time.Second); err != nil {
			return err
		}
		c := `nohup '` + runtimePath + `/mavo-pcm-bridge.armv7' --voice-route-session --verbose </dev/null >'` + runtimePath + `/route.log' 2>&1 & pid=$!; start=$(cut -d ' ' -f 22 "/proc/$pid/stat"); printf '%s %s\n' "$pid" "$start" >'` + runtimePath + `/route.pid'`
		if err := runtimeShell(a, c, 8*time.Second); err != nil {
			return err
		}
		return nil
	case "runtime-status":
		if err := runtimeShell(a, "cat '"+runtimePath+"/route.log'; cat /sys/class/android_usb/f_audio/audio_enable; cat /proc/asound/card0/pcm4p/sub0/status; cat /proc/asound/card0/pcm4c/sub0/status", 8*time.Second); err != nil {
			return err
		}
		return runtimeShell(a, runtimeRouteReady(), 8*time.Second)
	case "runtime-stop":
		if err := runtimeShell(a, runtimeRouteOwned(), 8*time.Second); err != nil {
			return err
		}
		c := `read pid start < '` + runtimePath + `/route.pid'; kill -TERM "$pid"; n=0; while kill -0 "$pid" 2>/dev/null && test "$n" -lt 100; do sleep 0.1; n=$((n+1)); done; ! kill -0 "$pid" 2>/dev/null`
		err = runtimeShell(a, c, 15*time.Second)
		_ = runtimeShell(a, `cat '`+runtimePath+`/route.log'; cat /sys/class/android_usb/f_audio/audio_enable`, 8*time.Second)
		return err
	default:
		return fmt.Errorf("unknown runtime step")
	}
}
func runtimeRouteOwned() string {
	return `test -s '` + runtimePath + `/route.pid' && read pid start < '` + runtimePath + `/route.pid' && test "$(cut -d ' ' -f 22 "/proc/$pid/stat")" = "$start" && test "$(tr '\000' '\n' < "/proc/$pid/cmdline" | sed -n '1p')" = '` + runtimePath + `/mavo-pcm-bridge.armv7' && tr '\000' '\n' < "/proc/$pid/cmdline" | grep -q '^--voice-route-session$'`
}
func runtimeRouteReady() string {
	return runtimeRouteOwned() + ` && grep -q 'VoLTE route session active on hw:0,4' '` + runtimePath + `/route.log' && test "$(cat /sys/class/android_usb/f_audio/audio_enable)" = 1 && grep -q '^state: RUNNING' /proc/asound/card0/pcm4p/sub0/status && grep -q '^state: RUNNING' /proc/asound/card0/pcm4c/sub0/status`
}
func runtimeCheckIdle() error {
	out, err := at("AT+CLCC")
	if err != nil || !atProbeSucceeded(out) || atResponseIsError(out) {
		return fmt.Errorf("cannot verify voice idle")
	}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "+CLCC:") {
			continue
		}
		var i, d, s, m, p int
		n, _ := fmt.Sscanf(line, "+CLCC: %d,%d,%d,%d,%d", &i, &d, &s, &m, &p)
		if n != 5 || m != 1 {
			return fmt.Errorf("voice call present or unknown; refusing runtime change")
		}
	}
	return nil
}
