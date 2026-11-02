//go:build darwin && cgo

package main

/*
#cgo LDFLAGS: -framework CoreAudio -framework CoreFoundation
#include "audio_darwin.h"
*/
import "C"
import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
	"unsafe"
)

var stopOnce sync.Once
var audioStop = make(chan struct{})

func stopAudio() { stopOnce.Do(func() { close(audioStop) }) }
func streamAudio(location uint32) error {
	if rc := C.vx_audio_start(C.uint32_t(location)); rc != 0 {
		return fmt.Errorf("无法打开模组音频端点: %d", rc)
	}
	defer C.vx_audio_stop()
	ended := make(chan error, 1)
	go func() {
		for {
			h := make([]byte, 4)
			if _, e := io.ReadFull(os.Stdin, h); e != nil {
				ended <- e
				return
			}
			n := int(binary.LittleEndian.Uint32(h))
			if n < 2 || n > 3200 || n%2 != 0 {
				ended <- fmt.Errorf("invalid PCM frame")
				return
			}
			b := make([]byte, n)
			if _, e := io.ReadFull(os.Stdin, b); e != nil {
				ended <- e
				return
			}
			samples := make([]int16, n/2)
			for i := range samples {
				samples[i] = int16(binary.LittleEndian.Uint16(b[i*2:]))
			}
			C.vx_audio_write((*C.int16_t)(unsafe.Pointer(&samples[0])), C.int(len(samples)))
		}
	}()
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()
	samples := make([]int16, 1600)
	for {
		select {
		case <-audioStop:
			return nil
		case e := <-ended:
			if e == io.EOF {
				return nil
			}
			return e
		case <-tick.C:
			n := int(C.vx_audio_read((*C.int16_t)(unsafe.Pointer(&samples[0])), C.int(len(samples))))
			if n == 0 {
				continue
			}
			b := make([]byte, 4+n*2)
			binary.LittleEndian.PutUint32(b, uint32(n*2))
			for i := 0; i < n; i++ {
				binary.LittleEndian.PutUint16(b[4+i*2:], uint16(samples[i]))
			}
			if _, e := os.Stdout.Write(b); e != nil {
				return e
			}
		}
	}
}
