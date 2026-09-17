package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

const coreListen = "127.0.0.1:7576"

var configMu sync.Mutex

type coreConfig struct {
	Server  map[string]any   `yaml:"server"`
	Web     map[string]any   `yaml:"web"`
	Devices []map[string]any `yaml:"devices"`
}

func writeAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".vohivex-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if _, err = tmp.Write(data); err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(name, path)
}

func replaceSectionScalar(source, section, key, value string) (string, error) {
	lines := strings.SplitAfter(source, "\n")
	sectionRE := regexp.MustCompile(`^` + regexp.QuoteMeta(section) + `\s*:\s*(?:#.*)?(?:\r?\n)?$`)
	keyRE := regexp.MustCompile(`^(\s+)` + regexp.QuoteMeta(key) + `\s*:\s*([^#\r\n]*)(.*)$`)
	inSection := false
	for index, line := range lines {
		plain := strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
		if len(plain) > 0 && plain[0] != ' ' && plain[0] != '\t' {
			inSection = sectionRE.MatchString(line)
			continue
		}
		if !inSection {
			continue
		}
		ending := ""
		if strings.HasSuffix(line, "\r\n") {
			ending = "\r\n"
		} else if strings.HasSuffix(line, "\n") {
			ending = "\n"
		}
		base := strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
		match := keyRE.FindStringSubmatch(base)
		if match == nil {
			continue
		}
		quoted, _ := json.Marshal(value)
		comment := match[3]
		if strings.TrimSpace(comment) != "" && !strings.Contains(comment, "#") {
			comment = ""
		}
		lines[index] = match[1] + key + ": " + string(quoted) + comment + ending
		return strings.Join(lines, ""), nil
	}
	return "", fmt.Errorf("%s.%s scalar was not found", section, key)
}

func readCoreConfig(path string) (*coreConfig, []byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	var cfg coreConfig
	if err := yaml.Unmarshal(raw, &cfg); err != nil || cfg.Web == nil {
		return nil, nil, errors.New("existing configuration is invalid")
	}
	return &cfg, raw, nil
}

func prepareConfig(path string) error {
	configMu.Lock()
	defer configMu.Unlock()
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		content := []byte("server:\n  port: \"127.0.0.1:7576\"\n\nweb:\n  username: \"admin\"\n  password: \"admin\"\n\ndevices: []\n")
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return err
		}
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return err
		}
		_, writeErr := file.Write(content)
		if closeErr := file.Close(); writeErr == nil {
			writeErr = closeErr
		}
		return writeErr
	}
	cfg, raw, err := readCoreConfig(path)
	if err != nil {
		return err
	}
	if fmt.Sprint(cfg.Server["port"]) == coreListen {
		return nil
	}
	updated, err := replaceSectionScalar(string(raw), "server", "port", coreListen)
	if err != nil {
		return errors.New("existing config needs a server.port scalar; refusing automatic migration")
	}
	var check coreConfig
	if err := yaml.Unmarshal([]byte(updated), &check); err != nil || fmt.Sprint(check.Server["port"]) != coreListen {
		return errors.New("configuration migration validation failed")
	}
	current, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(current, raw) {
		return errors.New("configuration changed during migration")
	}
	return writeAtomic(path, []byte(updated), 0o600)
}

func configUsername(path string) (string, error) {
	cfg, _, err := readCoreConfig(path)
	if err != nil {
		return "", err
	}
	value, ok := cfg.Web["username"].(string)
	if !ok || value == "" {
		return "", errors.New("username is missing")
	}
	return value, nil
}

func configCredentials(path string) (string, string, error) {
	cfg, _, err := readCoreConfig(path)
	if err != nil {
		return "", "", err
	}
	username, uok := cfg.Web["username"].(string)
	password, pok := cfg.Web["password"].(string)
	if !uok || !pok || username == "" || password == "" {
		return "", "", errors.New("web credentials are missing")
	}
	return username, password, nil
}

func renameConfigUsername(path, username string) error {
	if !regexp.MustCompile(`^[A-Za-z0-9_.@-]{3,64}$`).MatchString(username) {
		return errors.New("用户名需为 3–64 位字母、数字或 _ . @ -")
	}
	configMu.Lock()
	defer configMu.Unlock()
	_, raw, err := readCoreConfig(path)
	if err != nil {
		return err
	}
	updated, err := replaceSectionScalar(string(raw), "web", "username", username)
	if err != nil {
		return errors.New("配置格式不支持安全更新")
	}
	current, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(current, raw) {
		return errors.New("配置已变更，请刷新后重试")
	}
	if err := writeAtomic(path+".before-username", raw, 0o600); err != nil {
		return err
	}
	return writeAtomic(path, []byte(updated), 0o600)
}
