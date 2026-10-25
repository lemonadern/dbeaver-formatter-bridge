// Copyright 2026 lemonadern
// SPDX-License-Identifier: Apache-2.0

// Package instance reads DBeaver's Java properties instance registry.
package instance

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

type Instance struct {
	Port     int
	Password string
}

func Load(workspace string) (Instance, error) {
	data, err := os.ReadFile(filepath.Join(workspace, ".metadata", "dbeaver-instance.properties"))
	if err != nil {
		return Instance{}, fmt.Errorf("cannot read DBeaver instance registry; check --workspace and start DBeaver")
	}
	props, err := Parse(data)
	if err != nil {
		return Instance{}, fmt.Errorf("invalid DBeaver instance registry")
	}
	var found []Instance
	// Older DBeaver releases use unprefixed port/password properties.
	if p, ok := props["port"]; ok {
		if v, err := valid(p, props["password"]); err == nil {
			found = append(found, v)
		}
	}
	for k, p := range props {
		if !strings.HasPrefix(k, "instance.") || !strings.HasSuffix(k, ".port") {
			continue
		}
		pid := strings.TrimSuffix(strings.TrimPrefix(k, "instance."), ".port")
		if n, e := strconv.ParseInt(pid, 10, 64); e != nil || n <= 0 {
			continue
		}
		if v, err := valid(p, props[strings.TrimSuffix(k, "port")+"password"]); err == nil {
			found = append(found, v)
		}
	}
	if len(found) == 0 {
		return Instance{}, fmt.Errorf("no usable DBeaver instance; start DBeaver with the specified workspace")
	}
	if len(found) > 1 {
		return Instance{}, fmt.Errorf("multiple DBeaver instances registered; use a workspace with one running instance")
	}
	return found[0], nil
}
func valid(port, password string) (Instance, error) {
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 || strings.TrimSpace(password) == "" || strings.ContainsAny(password, "\r\n") {
		return Instance{}, fmt.Errorf("invalid instance")
	}
	return Instance{n, password}, nil
}

// Parse implements Properties.load(Reader) for DBeaver UTF-8 registries: logical lines,
// escaped separators, continuation whitespace, and UTF-16 unicode escapes.
func Parse(data []byte) (map[string]string, error) {
	if !utf8.Valid(data) {
		return nil, fmt.Errorf("instance registry is not UTF-8")
	}
	lines := strings.Split(strings.ReplaceAll(strings.ReplaceAll(string(data), "\r\n", "\n"), "\r", "\n"), "\n")
	out := map[string]string{}
	for i := 0; i < len(lines); i++ {
		line := strings.TrimLeft(lines[i], " \t\f")
		if line == "" || line[0] == '#' || line[0] == '!' {
			continue
		}
		for {
			count := 0
			for j := len(line) - 1; j >= 0 && line[j] == '\\'; j-- {
				count++
			}
			if count%2 == 0 {
				break
			}
			line = line[:len(line)-1]
			i++
			if i >= len(lines) {
				break
			}
			line += strings.TrimLeft(lines[i], " \t\f")
		}
		split := len(line)
		escaped := false
		for j, c := range line {
			if !escaped && (c == '=' || c == ':' || c == ' ' || c == '\t' || c == '\f') {
				split = j
				break
			}
			if c == '\\' {
				escaped = !escaped
			} else {
				escaped = false
			}
		}
		tail := strings.TrimLeft(line[split:], " \t\f")
		if len(tail) > 0 && (tail[0] == '=' || tail[0] == ':') {
			tail = strings.TrimLeft(tail[1:], " \t\f")
		}
		key, e := unescape(line[:split])
		if e != nil {
			return nil, e
		}
		val, e := unescape(tail)
		if e != nil {
			return nil, e
		}
		out[key] = val
	}
	return out, nil
}
func unescape(s string) (string, error) {
	r := []rune(s)
	var units []uint16
	for i := 0; i < len(r); i++ {
		c := r[i]
		if c == '\\' {
			i++
			if i == len(r) {
				break
			}
			c = r[i]
			switch c {
			case 't':
				c = '\t'
			case 'n':
				c = '\n'
			case 'r':
				c = '\r'
			case 'f':
				c = '\f'
			case 'u':
				if i+4 >= len(r) {
					return "", fmt.Errorf("invalid unicode escape")
				}
				n, e := strconv.ParseUint(string(r[i+1:i+5]), 16, 16)
				if e != nil {
					return "", e
				}
				c = rune(n)
				i += 4
			}
		}
		if c > 0xffff {
			units = append(units, utf16.Encode([]rune{c})...)
		} else {
			units = append(units, uint16(c))
		}
	}
	return string(utf16.Decode(units)), nil
}
