package main

import (
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func recorderFinished(recorder *activeCallRecorder) bool {
	select {
	case <-recorder.done:
		return true
	default:
		return false
	}
}

func callPCMFormat(format []byte) bool {
	return len(format) >= 16 && binary.LittleEndian.Uint16(format) == 1 &&
		binary.LittleEndian.Uint16(format[2:]) == 1 && binary.LittleEndian.Uint32(format[4:]) == 8000 &&
		binary.LittleEndian.Uint32(format[8:]) == 16000 && binary.LittleEndian.Uint16(format[12:]) == 2 &&
		binary.LittleEndian.Uint16(format[14:]) == 16
}

// arecord finalizes RIFF/data sizes only at close. While recording, validate
// the format and bytes actually written, not the provisional length fields.
// Samples can be silent; this proves capture progress, not audible call media.
func captureHasSamples(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	header := make([]byte, 12)
	if _, err := io.ReadFull(f, header); err != nil || string(header[:4]) != "RIFF" || string(header[8:]) != "WAVE" {
		return false
	}
	validFormat := false
	for position := int64(12); position+8 <= info.Size() && position < 1<<20; {
		chunk := make([]byte, 8)
		if _, err := f.ReadAt(chunk, position); err != nil {
			return false
		}
		length := int64(binary.LittleEndian.Uint32(chunk[4:]))
		position += 8
		if string(chunk[:4]) == "data" {
			return validFormat && info.Size()-position >= 3200 // 200 ms of real PCM bytes
		}
		if length > info.Size()-position {
			return false
		}
		if string(chunk[:4]) == "fmt " {
			if length < 16 {
				return false
			}
			format := make([]byte, 16)
			if _, err := f.ReadAt(format, position); err != nil || !callPCMFormat(format) {
				return false
			}
			validFormat = true
		}
		position += length + length%2
	}
	return false
}

func awaitRecorderSamples(recorder *activeCallRecorder, timeout time.Duration) error {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for {
		if recorderFinished(recorder) {
			return errors.New("录音进程已退出，请检查模组音频通道")
		}
		if captureHasSamples(recorder.tempPath) {
			return nil
		}
		select {
		case <-recorder.done:
			return errors.New("录音进程已退出，请检查模组音频通道")
		case <-deadline.C:
			return errors.New("录音设备未返回 PCM 数据，请检查模组音频驱动与通话路由")
		case <-ticker.C:
		}
	}
}

// This only stops our capture process; it never sends a modem hangup command.
func stopRecorderProcess(recorder *activeCallRecorder) {
	if recorder.cmd.Process != nil && !recorderFinished(recorder) {
		_ = recorder.cmd.Process.Signal(os.Interrupt)
	}
	select {
	case <-recorder.done:
	case <-time.After(5 * time.Second):
		_ = recorder.cmd.Process.Kill()
		<-recorder.done
	}
}

// Docker may mask /proc/asound. Read the USB identity through sysfs, without
// opening an unrelated host microphone or changing the USB configuration.
func usbSoundIdentity(cardPath string) string {
	path, err := filepath.EvalSymlinks(cardPath)
	if err != nil {
		return ""
	}
	for depth := 0; depth < 8 && path != "/"; depth++ {
		vendor, vendorErr := os.ReadFile(filepath.Join(path, "idVendor"))
		product, productErr := os.ReadFile(filepath.Join(path, "idProduct"))
		if vendorErr == nil && productErr == nil {
			return strings.TrimSpace(string(vendor)) + ":" + strings.TrimSpace(string(product))
		}
		path = filepath.Dir(path)
	}
	return ""
}

// Validate actual audio data, not merely the existence of a WAV header. The
// recorder supports PCM mono S16_LE at 8 kHz; duration comes from samples.
func wavDuration(path string) (int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return 0, err
	}
	bad := errors.New("invalid PCM WAV")
	header := make([]byte, 12)
	if _, err := io.ReadFull(f, header); err != nil || string(header[:4]) != "RIFF" || string(header[8:]) != "WAVE" {
		return 0, bad
	}
	limit := int64(binary.LittleEndian.Uint32(header[4:8])) + 8
	if limit > info.Size() || limit < 44 {
		return 0, bad
	}
	validFormat := false
	var bytes int64
	for position := int64(12); position+8 <= limit; {
		chunk := make([]byte, 8)
		if _, err := f.ReadAt(chunk, position); err != nil {
			return 0, bad
		}
		length := int64(binary.LittleEndian.Uint32(chunk[4:]))
		position += 8
		if length > limit-position {
			return 0, bad
		}
		switch string(chunk[:4]) {
		case "fmt ":
			if length < 16 {
				return 0, bad
			}
			format := make([]byte, 16)
			if _, err := f.ReadAt(format, position); err != nil {
				return 0, bad
			}
			validFormat = callPCMFormat(format)
			if !validFormat {
				return 0, bad
			}
		case "data":
			if !validFormat || length == 0 || length%2 != 0 {
				return 0, bad
			}
			bytes += length
		}
		position += length + length%2
	}
	if bytes == 0 {
		return 0, bad
	}
	return bytes / 16000, nil
}
