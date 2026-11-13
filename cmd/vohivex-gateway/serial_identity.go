package main

import (
	"errors"
	"regexp"
	"strings"
)

var serialIMEILine = regexp.MustCompile(`^(?:\+CGSN:\s*)?(?:"([0-9]{15})"|([0-9]{15}))$`)

// Parse only complete lines. An identity must precede its terminating OK, and
// conflicting identity lines fail closed. Never include the modem reply in errors.
func parseSerialIdentity(response string) (string, bool, error) {
	end := strings.LastIndexAny(response, "\r\n")
	if end < 0 {
		return "", false, nil
	}
	identity := ""
	for _, line := range strings.FieldsFunc(response[:end+1], func(c rune) bool { return c == '\r' || c == '\n' }) {
		line = strings.TrimSpace(line)
		if line == "ERROR" || strings.HasPrefix(line, "+CME ERROR:") {
			return "", false, errors.New("modem rejected identity query")
		}
		if match := serialIMEILine.FindStringSubmatch(line); len(match) == 3 {
			value := match[1]
			if value == "" {
				value = match[2]
			}
			if identity != "" && identity != value {
				return "", false, errors.New("conflicting modem identity response")
			}
			identity = value
		}
		if line == "OK" && identity != "" {
			return identity, true, nil
		}
	}
	return "", false, nil
}
