package main

import "strings"

func atResponseComplete(resp string) bool {
	normalized := strings.ReplaceAll(resp, "\r\n", "\n")
	return strings.Contains(normalized, "\nOK\n") ||
		strings.HasSuffix(normalized, "\nOK") ||
		atResponseIsError(normalized)
}

func atResponseHasPrompt(resp string) bool {
	trimmed := strings.TrimRight(resp, " \t\r\n")
	return strings.HasSuffix(trimmed, ">")
}

func atResponseIsError(resp string) bool {
	normalized := strings.ToUpper(strings.ReplaceAll(resp, "\r\n", "\n"))
	return strings.Contains(normalized, "\nERROR\n") ||
		strings.HasSuffix(normalized, "\nERROR") ||
		strings.Contains(normalized, "+CME ERROR:") ||
		strings.Contains(normalized, "+CMS ERROR:")
}

// A probe must receive OK. ERROR merely proves that a bulk interface accepted
// bytes; it is not the modem's AT channel (the QMI interface can do that).
func atProbeSucceeded(resp string) bool {
	normalized := strings.ReplaceAll(strings.TrimSpace(resp), "\r\n", "\n")
	return normalized == "OK" || strings.HasSuffix(normalized, "\nOK")
}
