//go:build darwin && cgo

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestVoiceCommandsRejectConfigurationAndInjection(t *testing.T) {
	for _, s := range []string{"AT+CLCC", "ATA", "ATH", "ATD+12025550123;", `AT+VTS="1"`, `AT+CLDTMF=1,"1,2,#"`} {
		if !voiceAT.MatchString(s) {
			t.Fatalf("valid command rejected: %s", s)
		}
	}
	for _, s := range []string{"AT+CFUN=1,1", "ATD123;\rATH", "ATD*99#;", "AT+QCFG=\"ims\",0", "ATD123;ATH", "ATD;"} {
		if voiceAT.MatchString(s) {
			t.Fatalf("unsafe command allowed: %s", s)
		}
	}
}
func TestRollbackBoundToPhysicalDeviceAndKnownUSB(t *testing.T) {
	t.Setenv("VOHIVEX_CALL_BRIDGE_STATE", t.TempDir())
	location = 0x2220000
	for _, s := range []savedUSB{{Location: 1, Original: originalUSB}, {Location: location, Original: enabledUSB}} {
		b, _ := json.Marshal(s)
		os.WriteFile(snapshotFile(), b, 0600)
		if _, e := saved(); e == nil {
			t.Fatal("mismatched rollback accepted")
		}
	}
	b, _ := json.Marshal(savedUSB{Location: location, Original: originalUSB})
	os.WriteFile(snapshotFile(), b, 0600)
	if _, e := saved(); e != nil {
		t.Fatal(e)
	}
}
func TestRuntimeRejectsUnverifiedBinaries(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("VOHIVEX_CALL_RUNTIME_DIR", dir)
	if e := verifyRuntimeFiles(); e == nil {
		t.Fatal("missing runtime accepted")
	}
	for _, f := range runtimeFiles {
		os.WriteFile(filepath.Join(dir, f.name), make([]byte, f.size), 0600)
	}
	if e := verifyRuntimeFiles(); e == nil {
		t.Fatal("wrong hash accepted")
	}
}
