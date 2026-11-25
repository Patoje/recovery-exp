package main

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

func sha256hex(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// secretValue extracts a value from a configuration file without logging it.
// ini: "key = value" inside [section] (no section: before the first header);
// env: KEY=VALUE; raw: the whole file without the trailing newline.
func secretValue(content, format, section, key string) (string, bool) {
	switch format {
	case "raw":
		return strings.TrimRight(content, "\r\n"), true
	case "env":
		for _, l := range strings.Split(content, "\n") {
			k, v, ok := strings.Cut(strings.TrimSpace(l), "=")
			if ok && strings.TrimSpace(strings.TrimPrefix(k, "export ")) == key {
				return strings.Trim(strings.TrimSpace(v), `"'`), true
			}
		}
		return "", false
	}
	cur := ""
	for _, l := range strings.Split(content, "\n") {
		t := strings.TrimSpace(l)
		if strings.HasPrefix(t, "[") && strings.HasSuffix(t, "]") {
			cur = strings.TrimSpace(t[1 : len(t)-1])
			continue
		}
		if t == "" || strings.HasPrefix(t, ";") || strings.HasPrefix(t, "#") || cur != section {
			continue
		}
		k, v, ok := strings.Cut(t, "=")
		if ok && strings.TrimSpace(k) == key {
			return strings.TrimSpace(v), true
		}
	}
	return "", false
}
