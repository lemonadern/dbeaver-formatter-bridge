// Copyright 2026 lemonadern
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestPublicP2VerifiesIndexAndAssetBytes(t *testing.T) {
	files := map[string][]byte{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if data, ok := files[r.URL.Path]; ok {
			w.Write(data)
		} else {
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	expected := t.TempDir()
	index := []byte(fmt.Sprintf(`<repository><children><child location="%s/release/"/></children></repository>`, server.URL))
	for _, name := range []string{"p2.index", "compositeContent.xml", "compositeArtifacts.xml"} {
		data := index
		if name == "p2.index" {
			data = []byte("version=1\n")
		}
		if err := os.WriteFile(filepath.Join(expected, name), data, 0600); err != nil {
			t.Fatal(err)
		}
		files["/updates/"+name] = data
	}
	sums := ""
	proofFiles := map[string]string{}
	for _, name := range []string{"p2.index", "content.jar", "artifacts.jar", "plugin.jar", "feature.jar"} {
		data := []byte("verified " + name)
		files["/release/"+name] = data
		proofFiles[name] = fmt.Sprintf("%x", sha256.Sum256(data))
		sums += fmt.Sprintf("%x  %s\n", sha256.Sum256(data), name)
	}
	files["/release/SHA256SUMS"] = []byte(sums)
	proofFiles["SHA256SUMS"] = fmt.Sprintf("%x", sha256.Sum256([]byte(sums)))
	proof := map[string]map[string]string{server.URL + "/release/": proofFiles}
	if err := verifyP2URL(server.URL+"/updates/", expected, proof); err != nil {
		t.Fatal(err)
	}
	files["/release/plugin.jar"] = []byte("modified")
	if err := verifyP2URL(server.URL+"/updates/", expected, proof); err == nil {
		t.Fatal("accepted modified public JAR")
	}
	files["/updates/compositeContent.xml"] = []byte("stale index")
	if err := verifyP2URL(server.URL+"/updates/", expected, proof); err == nil {
		t.Fatal("accepted stale Pages index")
	}
}
