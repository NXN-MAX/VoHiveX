package main

import (
	"crypto/md5"
	"errors"
	"strings"
)

const qadbPassword = "SH_adb_quectel"

func parseQADBChallenge(response string) (string, error) {
	var challenge string
	for _, raw := range strings.Split(strings.ReplaceAll(response, "\r", ""), "\n") {
		line := strings.TrimSpace(raw)
		if !strings.HasPrefix(strings.ToUpper(line), "+QADBKEY:") {
			continue
		}
		if challenge != "" {
			return "", errors.New("duplicate QADBKEY challenge")
		}
		challenge = strings.TrimSpace(line[len("+QADBKEY:"):])
	}
	if len(challenge) != 8 {
		return "", errors.New("invalid QADBKEY challenge")
	}
	for _, character := range challenge {
		if character < '0' || character > '9' {
			return "", errors.New("invalid QADBKEY challenge")
		}
	}
	return challenge, nil
}

func deriveQADBKey(challenge string) (string, error) {
	if len(challenge) != 8 {
		return "", errors.New("invalid QADBKEY challenge")
	}
	password := []byte(qadbPassword)
	salt := []byte(challenge)
	for _, character := range salt {
		if character < '0' || character > '9' {
			return "", errors.New("invalid QADBKEY challenge")
		}
	}
	initial := append(append(append([]byte{}, password...), []byte("$1$")...), salt...)
	alternate := md5.Sum(append(append(append([]byte{}, password...), salt...), password...))
	for remaining := len(password); remaining > 0; remaining -= min(remaining, len(alternate)) {
		count := min(remaining, len(alternate))
		initial = append(initial, alternate[:count]...)
	}
	for length := len(password); length > 0; length >>= 1 {
		if length&1 == 1 {
			initial = append(initial, 0)
		} else {
			initial = append(initial, password[0])
		}
	}
	digestArray := md5.Sum(initial)
	digest := digestArray[:]
	for round := 0; round < 1000; round++ {
		input := make([]byte, 0, 64)
		if round&1 == 1 {
			input = append(input, password...)
		} else {
			input = append(input, digest...)
		}
		if round%3 != 0 {
			input = append(input, salt...)
		}
		if round%7 != 0 {
			input = append(input, password...)
		}
		if round&1 == 1 {
			input = append(input, digest...)
		} else {
			input = append(input, password...)
		}
		next := md5.Sum(input)
		digest = next[:]
	}
	alphabet := []byte("./0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz")
	encoded := make([]byte, 0, 22)
	append64 := func(high, middle, low byte, count int) {
		value := uint32(high)<<16 | uint32(middle)<<8 | uint32(low)
		for range count {
			encoded = append(encoded, alphabet[value&0x3f])
			value >>= 6
		}
	}
	append64(digest[0], digest[6], digest[12], 4)
	append64(digest[1], digest[7], digest[13], 4)
	append64(digest[2], digest[8], digest[14], 4)
	append64(digest[3], digest[9], digest[15], 4)
	append64(digest[4], digest[10], digest[5], 4)
	append64(0, 0, digest[11], 2)
	return string(encoded[:15]), nil
}
