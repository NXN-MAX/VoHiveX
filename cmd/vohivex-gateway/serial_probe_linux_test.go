//go:build linux

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func probePTY(t *testing.T) (int, string) {
	t.Helper()
	fd, err := unix.Open("/dev/ptmx", unix.O_RDWR|unix.O_NOCTTY|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = unix.Close(fd) })
	if err = unix.IoctlSetPointerInt(fd, unix.TIOCSPTLCK, 0); err != nil {
		t.Fatal(err)
	}
	number, err := unix.IoctlGetInt(fd, unix.TIOCGPTN)
	if err != nil {
		t.Fatal(err)
	}
	return fd, fmt.Sprintf("/dev/pts/%d", number)
}

func TestSerialIMEIReadOnlyQueryAndRestore(t *testing.T) {
	master, path := probePTY(t)
	slave, err := unix.Open(path, unix.O_RDWR|unix.O_NOCTTY|unix.O_NONBLOCK, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(slave)
	before, err := unix.IoctlGetTermios(slave, unix.TCGETS)
	if err != nil {
		t.Fatal(err)
	}
	commands := make(chan string, 1)
	go func() {
		buffer := make([]byte, 128)
		command := ""
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			n, _ := unix.Read(master, buffer)
			if n > 0 {
				command += string(buffer[:n])
			}
			if strings.Contains(command, "\r") {
				break
			}
			time.Sleep(time.Millisecond)
		}
		commands <- command
		_, _ = unix.Write(master, []byte("\r\n+CGSN: \""+strings.Repeat("1", 15)+"\"\r\nOK\r\n"))
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	imei, err := probeSerialIMEI(ctx, path)
	if err != nil || imei != strings.Repeat("1", 15) {
		t.Fatalf("probe failed: %q %v", imei, err)
	}
	if cmd := <-commands; cmd != "AT+CGSN\r" {
		t.Fatalf("unexpected command: %q", cmd)
	}
	after, err := unix.IoctlGetTermios(slave, unix.TCGETS)
	if err != nil || *before != *after {
		t.Fatalf("termios not restored: %v", err)
	}
	// Exclusive mode must be released so the modem core can open the port next.
	fd, err := unix.Open(path, unix.O_RDWR|unix.O_NOCTTY|unix.O_NONBLOCK, 0)
	if err != nil {
		t.Fatal(err)
	}
	unix.Close(fd)
}

func TestSerialIMEIRespectsBusyPortAndDeadline(t *testing.T) {
	_, path := probePTY(t)
	owner, err := unix.Open(path, unix.O_RDWR|unix.O_NOCTTY|unix.O_NONBLOCK, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(owner)
	if err = unix.Flock(owner, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	if _, err = probeSerialIMEI(context.Background(), path); !errors.Is(err, errSerialBusy) {
		t.Fatalf("lock ignored: %v", err)
	}
	unix.Flock(owner, unix.LOCK_UN)
	if err = unix.IoctlSetInt(owner, unix.TIOCEXCL, 0); err != nil {
		t.Fatal(err)
	}
	if _, err = probeSerialIMEI(context.Background(), path); !errors.Is(err, errSerialBusy) {
		t.Fatalf("exclusive port ignored: %v", err)
	}
	unix.IoctlSetInt(owner, unix.TIOCNXCL, 0)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err = probeSerialIMEI(ctx, path); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("deadline ignored: %v", err)
	}
	if time.Since(start) > time.Second {
		t.Fatal("probe did not stop promptly")
	}
}

func TestSerialIMEIRejectsRegularFilesAndSymlinks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not-a-port")
	if err := os.WriteFile(path, []byte("unchanged"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := probeSerialIMEI(context.Background(), path); err == nil {
		t.Fatal("regular file accepted")
	}
	_, tty := probePTY(t)
	link := filepath.Join(t.TempDir(), "symlink")
	if err := os.Symlink(tty, link); err != nil {
		t.Fatal(err)
	}
	if _, err := probeSerialIMEI(context.Background(), link); err == nil {
		t.Fatal("symlink accepted")
	}
	raw, _ := os.ReadFile(path)
	if string(raw) != "unchanged" {
		t.Fatal("regular file was modified")
	}
}
