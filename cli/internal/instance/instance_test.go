// Copyright 2026 lemonadern
// SPDX-License-Identifier: Apache-2.0

package instance

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProperties(t *testing.T) {
	got, err := Parse([]byte("  # comment\r\nport: 1234\r\npassword = a\\:b\\=c\\\\d\\\r\n  e\\t\\u65e5\\uD83D\\uDE80\nkey\\ with\\ space value\nlatin=é\n"))
	if err != nil {
		t.Fatal(err)
	}
	for k, want := range map[string]string{"port": "1234", "password": "a:b=c\\de\t日🚀", "key with space": "value", "latin": "é"} {
		if got[k] != want {
			t.Errorf("%s = %q want %q", k, got[k], want)
		}
	}
	for _, bad := range []string{`a=\u12`, `a=\uZZZZ`} {
		if _, err := Parse([]byte(bad)); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
}
func TestLoad(t *testing.T) {
	for _, tc := range []struct {
		props string
		ok    bool
	}{
		{"port=1234\npassword=secret", true}, {"instance.42.port=1234\ninstance.42.password=secret\ninstance.42.startedAt=100", true},
		{"port=1234", false}, {"port=70000\npassword=secret", false}, {"instance.42.port=1234\ninstance.42.password=a\ninstance.43.port=1235\ninstance.43.password=b", false},
	} {
		dir := t.TempDir()
		os.Mkdir(filepath.Join(dir, ".metadata"), 0700)
		os.WriteFile(filepath.Join(dir, ".metadata", "dbeaver-instance.properties"), []byte(tc.props), 0600)
		_, err := Load(dir)
		if (err == nil) != tc.ok {
			t.Fatalf("props %q error %v", tc.props, err)
		}
	}
}

func TestUTF8Registry(t *testing.T) {
	p, err := Parse([]byte("日本語=直接の値\npassword=é🚀"))
	if err != nil || p["日本語"] != "直接の値" || p["password"] != "é🚀" {
		t.Fatalf("UTF-8 parse %v %v", p, err)
	}
	if _, err := Parse([]byte{'x', '=', 0xff}); err == nil {
		t.Fatal("accepted invalid UTF-8")
	}
}
