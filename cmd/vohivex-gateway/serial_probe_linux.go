//go:build linux

package main

import (
	"context"
	"errors"
	"strings"

	"golang.org/x/sys/unix"
)

func serialPoll(ctx context.Context, fd int, event int16) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		poll := []unix.PollFd{{Fd: int32(fd), Events: event}}
		_, err := unix.Poll(poll, 100)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return err
		}
		if poll[0].Revents&event != 0 {
			return nil
		}
		if poll[0].Revents&(unix.POLLERR|unix.POLLHUP|unix.POLLNVAL) != 0 {
			return errors.New("serial device disconnected")
		}
	}
}

// This probe sends only the read-only hardware identity query. It never dials,
// changes USB mode, or sends SMS. Keep the reply out of logs and error messages.
func probeSerialIMEI(ctx context.Context, path string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	fd, err := unix.Open(path, unix.O_RDWR|unix.O_NOCTTY|unix.O_NONBLOCK|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if errors.Is(err, unix.EBUSY) {
		return "", errSerialBusy
	}
	if err != nil {
		return "", err
	}
	defer unix.Close(fd)
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil || stat.Mode&unix.S_IFMT != unix.S_IFCHR {
		return "", errors.New("AT port is not a character device")
	}
	if err := unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		if errors.Is(err, unix.EWOULDBLOCK) {
			return "", errSerialBusy
		}
		return "", err
	}
	defer unix.Flock(fd, unix.LOCK_UN)
	if err := unix.IoctlSetInt(fd, unix.TIOCEXCL, 0); err != nil {
		return "", err
	}
	defer unix.IoctlSetInt(fd, unix.TIOCNXCL, 0)
	previous, err := unix.IoctlGetTermios(fd, unix.TCGETS)
	if err != nil {
		return "", err
	}
	settings := *previous
	settings.Iflag, settings.Oflag, settings.Lflag = 0, 0, 0
	settings.Cflag = unix.CS8 | unix.CREAD | unix.CLOCAL | unix.B115200
	settings.Cc[unix.VMIN], settings.Cc[unix.VTIME] = 0, 0
	if err := unix.IoctlSetTermios(fd, unix.TCSETS, &settings); err != nil {
		return "", err
	}
	defer unix.IoctlSetTermios(fd, unix.TCSETS, previous)
	if err := unix.IoctlSetInt(fd, unix.TCFLSH, unix.TCIFLUSH); err != nil {
		return "", err
	}
	command := []byte("AT+CGSN\r")
	for len(command) > 0 {
		if err := serialPoll(ctx, fd, unix.POLLOUT); err != nil {
			return "", err
		}
		written, err := unix.Write(fd, command)
		if errors.Is(err, unix.EAGAIN) || errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return "", err
		}
		if written == 0 {
			return "", errors.New("serial write made no progress")
		}
		command = command[written:]
	}
	var response strings.Builder
	buffer := make([]byte, 256)
	for response.Len() < 4096 {
		if err := serialPoll(ctx, fd, unix.POLLIN); err != nil {
			return "", err
		}
		n, err := unix.Read(fd, buffer)
		if errors.Is(err, unix.EAGAIN) || errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return "", err
		}
		if n == 0 {
			return "", errors.New("serial device disconnected")
		}
		if response.Len()+n > 4096 {
			return "", errors.New("serial identity response exceeded limit")
		}
		response.Write(buffer[:n])
		identity, complete, err := parseSerialIdentity(response.String())
		if err != nil {
			return "", err
		}
		if complete {
			return identity, nil
		}
	}
	return "", errors.New("serial identity response exceeded limit")
}
