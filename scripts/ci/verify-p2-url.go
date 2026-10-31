//go:build ignore

// Copyright 2026 lemonadern
// SPDX-License-Identifier: Apache-2.0

// Check the deployed composite against the prepared index, then download and
// checksum every asset of its children through the public HTTP URLs.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	base := flag.String("url", "", "public composite repository URL")
	expected := flag.String("expected", "build/pages/updates", "prepared composite directory")
	proofFile := flag.String("proof", "build/pages-proof.json", "API-verified release hashes, kept outside the published site")
	flag.Parse()
	proofData, err := os.ReadFile(*proofFile)
	var proof map[string]map[string]string
	if err == nil {
		err = json.Unmarshal(proofData, &proof)
	}
	if err != nil || len(proof) == 0 {
		fmt.Fprintln(os.Stderr, "missing verified release proof")
		os.Exit(1)
	}
	if err := verifyP2URL(*base, *expected, proof); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("PASS: public p2 index and release asset checksums")
}

func readHTTP(ctx context.Context, address string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", "openai-codex/1.0")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("p2 HTTP request failed")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("p2 HTTP status %d", response.StatusCode)
	}
	const limit = 16 << 20
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil || len(data) > limit {
		return nil, fmt.Errorf("p2 response incomplete or too large")
	}
	return data, nil
}

func verifyP2URL(base, expected string, proof map[string]map[string]string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	parsed, err := url.Parse(base)
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || !strings.HasSuffix(parsed.Path, "/") {
		return fmt.Errorf("expected HTTP repository URL ending in /")
	}
	var children struct {
		Children struct {
			Items []struct {
				Location string `xml:"location,attr"`
			} `xml:"child"`
		} `xml:"children"`
	}
	for _, name := range []string{"p2.index", "compositeContent.xml", "compositeArtifacts.xml"} {
		want, err := os.ReadFile(filepath.Join(expected, name))
		if err != nil {
			return err
		}
		got, err := readHTTP(ctx, base+name)
		if err != nil {
			return err
		}
		if !bytes.Equal(got, want) {
			return fmt.Errorf("deployed p2 index differs: %s", name)
		}
		if name == "compositeArtifacts.xml" {
			if err := xml.Unmarshal(want, &children); err != nil {
				return err
			}
		}
	}
	if len(children.Children.Items) == 0 {
		return fmt.Errorf("empty p2 repository")
	}
	if len(children.Children.Items) != len(proof) {
		return fmt.Errorf("public children differ from verified release proof")
	}
	for _, child := range children.Children.Items {
		childURL, err := parsed.Parse(child.Location)
		if err != nil {
			return err
		}
		location := strings.TrimSuffix(childURL.String(), "/") + "/"
		expectedFiles, ok := proof[location]
		if !ok {
			return fmt.Errorf("missing verified child proof")
		}
		sums, err := readHTTP(ctx, location+"SHA256SUMS")
		if err != nil {
			return err
		}
		sumHash := sha256.Sum256(sums)
		if hex.EncodeToString(sumHash[:]) != expectedFiles["SHA256SUMS"] {
			return fmt.Errorf("public checksum list differs from verified release")
		}
		count := 0
		seen := map[string]bool{}
		for _, line := range strings.Split(strings.TrimSpace(string(sums)), "\n") {
			parts := strings.SplitN(line, "  ", 2)
			if len(parts) != 2 || len(parts[0]) != 64 || filepath.Base(parts[1]) != parts[1] || strings.ContainsAny(parts[1], "/\\?#%\r\t") || parts[1] == "." || seen[parts[1]] {
				return fmt.Errorf("invalid release checksum list")
			}
			if _, err := hex.DecodeString(parts[0]); err != nil {
				return err
			}
			if expectedFiles[parts[1]] != parts[0] {
				return fmt.Errorf("public file differs from verified release proof")
			}
			seen[parts[1]] = true
			data, err := readHTTP(ctx, location+parts[1])
			if err != nil {
				return err
			}
			sum := sha256.Sum256(data)
			if hex.EncodeToString(sum[:]) != parts[0] {
				return fmt.Errorf("public release checksum mismatch")
			}
			count++
		}
		for _, name := range []string{"p2.index", "content.jar", "artifacts.jar"} {
			if !seen[name] {
				return fmt.Errorf("missing public p2 file: %s", name)
			}
		}
		if count < 5 || count != len(expectedFiles)-1 {
			return fmt.Errorf("incomplete public p2 repository")
		}
	}
	return nil
}
