package main

import (
	"strings"
	"testing"
)

func TestSerialIdentityRequiresOneCompleteTerminatedReply(t *testing.T) {
	one, two := strings.Repeat("1", 15), strings.Repeat("2", 15)
	for _, tc := range []struct {
		name, response      string
		complete, wantError bool
	}{
		{"plain", "AT+CGSN\r\n" + one + "\r\nOK\r\n", true, false},
		{"quoted", "+CGSN: \"" + one + "\"\r\nOK\r\n", true, false},
		{"partial terminal", one + "\r\nOK", false, false},
		{"stale OK", "OK\r\n" + one + "\r\n", false, false},
		{"partial identity", one, false, false},
		{"unbalanced quote", "\"" + one + "\r\nOK\r\n", false, false},
		{"multiple identities", one + "\r\n" + two + "\r\nOK\r\n", false, true},
		{"error", one + "\r\nERROR\r\n", false, true},
		{"CME error", "+CME ERROR: test\r\n", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			identity, complete, err := parseSerialIdentity(tc.response)
			if complete != tc.complete || (err != nil) != tc.wantError {
				t.Fatalf("complete=%v err=%v", complete, err)
			}
			if complete && identity != one {
				t.Fatal("incorrect identity")
			}
			if err != nil && (strings.Contains(err.Error(), one) || strings.Contains(err.Error(), two)) {
				t.Fatal("error leaked modem identity")
			}
		})
	}
}
