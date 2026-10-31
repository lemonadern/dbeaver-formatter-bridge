// Copyright 2026 lemonadern
// SPDX-License-Identifier: Apache-2.0
package p2site

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"encoding/pem"
	"encoding/xml"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestP2DirectorIntegration installs the actual legacy release and updates it
// through HTTP composite metadata in a separate copy of the DBeaver product.
// It is opt-in: regular unit tests never touch a user's product installation.
func TestP2DirectorIntegration(t *testing.T) {
	home, legacyPath := os.Getenv("P2_DBEAVER_HOME"), os.Getenv("P2_LEGACY_ZIP")
	if home == "" || legacyPath == "" {
		t.Skip("set P2_DBEAVER_HOME and P2_LEGACY_ZIP to test a disposable product copy")
	}
	legacy, err := os.ReadFile(legacyPath)
	if err != nil {
		t.Fatal(err)
	}
	checksum := sha256.Sum256(legacy)
	if hex.EncodeToString(checksum[:]) != "a58dc4eee8b9ff2b3988d3621ef899036f1a40fb22642dc3691975be5464e112" {
		t.Fatal("integration fixture must be the exact public v0.1.0 p2 ZIP")
	}
	scratch := t.TempDir()
	product := filepath.Join(scratch, "product")
	if runtime.GOOS == "darwin" {
		product = filepath.Join(scratch, "Eclipse")
	}
	if err := copyProduct(home, product); err != nil {
		t.Fatal(err)
	}
	if err := normalizeProfiles(product); err != nil {
		t.Fatal(err)
	}
	launchers, err := filepath.Glob(filepath.Join(product, "plugins", "org.eclipse.equinox.launcher_*.jar"))
	if err != nil || len(launchers) != 1 {
		t.Fatal("expected one Equinox launcher")
	}
	java := filepath.Join(os.Getenv("JAVA_HOME"), "bin", "java")
	if runtime.GOOS == "windows" {
		java += ".exe"
	}
	if os.Getenv("JAVA_HOME") == "" {
		java = filepath.Join(home, "jre", "Contents", "Home", "bin", "java")
	}
	if _, err := os.Stat(java); err != nil {
		t.Fatal("set JAVA_HOME to an existing JDK")
	}
	var requested = map[string]int{}
	var requestMu sync.Mutex
	var current map[string][]byte
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Each director invocation is sequential, and all requests finish before the
		// next invocation mutates the maps.
		requestMu.Lock()
		requested[r.URL.Path]++
		defer requestMu.Unlock()
		b, ok := current[strings.TrimPrefix(r.URL.Path, "/")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
		w.Write(b)
	}))
	defer server.Close()
	certPath := filepath.Join(scratch, "test-ca.pem")
	if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	trustStore := filepath.Join(scratch, "test-truststore")
	keytool := filepath.Join(filepath.Dir(java), "keytool")
	if runtime.GOOS == "windows" {
		keytool += ".exe"
	}
	importCert := exec.Command(keytool, "-importcert", "-noprompt", "-alias", "p2-local-test", "-file", certPath, "-keystore", trustStore, "-storepass", "p2-local-test")
	if output, err := importCert.CombinedOutput(); err != nil {
		t.Fatalf("test truststore: %v %s", err, output)
	}
	oldFlat, err := Flatten(legacy, "0.1.0", server.URL+"/old/")
	if err != nil {
		t.Fatal(err)
	}
	newZIP := bumpFixture(t, legacy, "0.1.0", "0.1.1")
	newer, err := Flatten(newZIP, "0.1.1", server.URL+"/new/")
	if err != nil {
		t.Fatal(err)
	}
	requestMu.Lock()
	current = map[string][]byte{}
	for name, data := range oldFlat {
		current["old/"+name] = data
	}
	for name, data := range newer {
		current["new/"+name] = data
	}
	index, err := Composite([]string{server.URL + "/old/"})
	if err != nil {
		t.Fatal(err)
	}
	for name, data := range index {
		current["updates/"+name] = data
	}
	requestMu.Unlock()
	run := func(action string) {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		args := []string{"-Djavax.net.ssl.trustStore=" + trustStore, "-Djavax.net.ssl.trustStorePassword=p2-local-test", "-Duser.home=" + filepath.Join(scratch, "user-home"), "-jar", launchers[0], "-nosplash", "-application", "org.eclipse.equinox.p2.director", "-repository", server.URL + "/updates/", "-installIU", pluginID + ".feature.feature.group", "-destination", product, "-profile", "DefaultProfile", "-configuration", filepath.Join(product, "configuration"), "-data", filepath.Join(scratch, "workspace")}
		if strings.HasPrefix(action, "update") {
			args = append(args, "-uninstallIU", pluginID+".feature.feature.group/0.1.0")
		}
		cmd := exec.CommandContext(ctx, java, args...)
		cmd.Dir = product
		cmd.Env = append(os.Environ(), "DBEAVER_DATA="+filepath.Join(scratch, "data"))
		output, err := cmd.CombinedOutput()
		if err != nil {
			log, _ := os.ReadFile(filepath.Join(scratch, "workspace", ".metadata", ".log"))
			t.Fatalf("%s failed: %v\n%s\n%s", action, err, output, log)
		}
		t.Logf("%s: %s", action, output)
	}
	run("install public 0.1.0 over HTTP")
	checkInstalled := func(version string) {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(product, "configuration", "org.eclipse.equinox.simpleconfigurator", "bundles.info"))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(data, []byte(pluginID+","+version+",")) {
			t.Fatalf("expected installed plugin %s", version)
		}
	}
	checkInstalled("0.1.0")
	index, err = Composite([]string{server.URL + "/old/", server.URL + "/new/"})
	if err != nil {
		t.Fatal(err)
	}
	requestMu.Lock()
	for name, data := range index {
		current["updates/"+name] = data
	}
	requestMu.Unlock()
	run("update to isolated fixture 0.1.1 over HTTP")
	checkInstalled("0.1.1")
	requestMu.Lock()
	defer requestMu.Unlock()
	for _, name := range []string{"/old/content.xml.xz", "/old/artifacts.xml.xz", "/new/content.xml.xz", "/new/artifacts.xml.xz"} {
		if requested[name] == 0 {
			t.Fatalf("XZ metadata not requested: %s", name)
		}
	}
	for _, name := range []string{"/old/content.jar", "/old/artifacts.jar", "/new/content.jar", "/new/artifacts.jar"} {
		if requested[name] > 0 {
			t.Fatalf("p2 fell back from generated XZ to JAR: %s", name)
		}
	}
	for _, path := range []string{"/old/" + pluginID + "_0.1.0.jar", "/new/" + pluginID + "_0.1.1.jar"} {
		if requested[path] == 0 {
			t.Fatalf("p2 did not fetch JAR %s", path)
		}
	}
}
func copyProduct(from, to string) error {
	return filepath.WalkDir(from, func(source string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(from, source)
		if err != nil {
			return err
		}
		if relative == "jre" && entry.IsDir() {
			return filepath.SkipDir
		}
		target := filepath.Join(to, relative)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm())
		}
		if info.Mode()&os.ModeSymlink != 0 {
			// A copied absolute or escaping relative link could redirect p2
			// writes to the source product. Reject links rather than trying
			// to infer which product paths will be mutated by Equinox.
			return fmt.Errorf("symbolic links are not permitted in a disposable product copy: %s", relative)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("unexpected product file type")
		}
		in, err := os.Open(source)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm())
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(out, in)
		closeErr := out.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
}
func bumpFixture(t *testing.T, archive []byte, from, to string) []byte {
	t.Helper()
	files, err := readZIP(archive)
	if err != nil {
		t.Fatal(err)
	}
	for _, prefix := range []string{"plugins/", "features/"} {
		id := pluginID
		if prefix == "features/" {
			id += ".feature"
		}
		name := prefix + id + "_" + from + ".jar"
		contents, err := readZIP(files[name])
		if err != nil {
			t.Fatal(err)
		}
		for key, b := range contents {
			if strings.HasSuffix(key, ".xml") || strings.HasSuffix(key, ".MF") {
				contents[key] = bytes.ReplaceAll(b, []byte(from), []byte(to))
			}
		}
		delete(files, name)
		files[prefix+id+"_"+to+".jar"] = sortedZIP(t, contents)
	}
	for _, kind := range []string{"content", "artifacts"} {
		x, err := metadataXML(files[kind+".jar"], kind+".xml")
		if err != nil {
			t.Fatal(err)
		}
		x = bytes.ReplaceAll(x, []byte(from), []byte(to))
		if kind == "artifacts" {
			x = fixtureChecksums(t, x, files, to)
		}
		files[kind+".jar"], err = metadataJAR(kind+".xml", x)
		if err != nil {
			t.Fatal(err)
		}
		files[kind+".xml.xz"] = encodeXZ(x)
	}
	return sortedZIP(t, files)
}
func sortedZIP(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	names := []string{}
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		w, err := z.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = w.Write(files[name]); err != nil {
			t.Fatal(err)
		}
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func fixtureChecksums(t *testing.T, data []byte, files map[string][]byte, version string) []byte {
	t.Helper()
	dec := xml.NewDecoder(bytes.NewReader(data))
	var b bytes.Buffer
	enc := xml.NewEncoder(&b)
	var jar []byte
	for {
		token, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if start, ok := token.(xml.StartElement); ok {
			if start.Name.Local == "artifact" {
				id := ""
				for _, a := range start.Attr {
					if a.Name.Local == "id" {
						id = a.Value
					}
				}
				prefix := "plugins/"
				if strings.HasSuffix(id, ".feature") {
					prefix = "features/"
				}
				jar = files[prefix+id+"_"+version+".jar"]
			}
			if start.Name.Local == "property" && jar != nil {
				name := ""
				for _, a := range start.Attr {
					if a.Name.Local == "name" {
						name = a.Value
					}
				}
				value := ""
				a := sha256.Sum256(jar)
				c := sha512.Sum512(jar)
				switch name {
				case "artifact.size", "download.size":
					value = strconv.Itoa(len(jar))
				case "download.checksum.sha-256":
					value = hex.EncodeToString(a[:])
				case "download.checksum.sha-512":
					value = hex.EncodeToString(c[:])
				}
				if value != "" {
					for i := range start.Attr {
						if start.Attr[i].Name.Local == "value" {
							start.Attr[i].Value = value
						}
					}
				}
			}
			token = start
		}
		if end, ok := token.(xml.EndElement); ok && end.Name.Local == "artifact" {
			jar = nil
		}
		if err := enc.EncodeToken(token); err != nil {
			t.Fatal(err)
		}
	}
	if err := enc.Flush(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func normalizeProfiles(product string) error {
	profiles, err := filepath.Glob(filepath.Join(product, "p2", "org.eclipse.equinox.p2.engine", "profileRegistry", "DefaultProfile.profile", "*.profile.gz"))
	if err != nil {
		return err
	}
	for _, name := range profiles {
		in, err := os.Open(name)
		if err != nil {
			return err
		}
		gz, err := gzip.NewReader(in)
		if err != nil {
			in.Close()
			return err
		}
		data, err := io.ReadAll(gz)
		gz.Close()
		in.Close()
		if err != nil {
			return err
		}
		for _, key := range []string{"org.eclipse.equinox.p2.cache", "org.eclipse.equinox.p2.installFolder"} {
			re := regexp.MustCompile(`(<property name='` + regexp.QuoteMeta(key) + `' value=')[^']*(')`)
			data = re.ReplaceAll(data, []byte("${1}"+filepath.ToSlash(product)+"${2}"))
		}
		data = bytes.ReplaceAll(data, []byte("name='org.eclipse.equinox.p2.roaming' value='true'"), []byte("name='org.eclipse.equinox.p2.roaming' value='false'"))
		out, err := os.Create(name)
		if err != nil {
			return err
		}
		zw := gzip.NewWriter(out)
		_, err = zw.Write(data)
		if err != nil {
			zw.Close()
			out.Close()
			return err
		}
		if err = zw.Close(); err != nil {
			out.Close()
			return err
		}
		if err = out.Close(); err != nil {
			return err
		}
	}
	return nil
}

func TestCopyProductRejectsSourceLinks(t *testing.T) {
	for _, name := range []string{"configuration", "p2", "plugins/bundle.jar"} {
		t.Run(strings.ReplaceAll(name, "/", "-"), func(t *testing.T) {
			scratch := t.TempDir()
			source := filepath.Join(scratch, "source")
			outside := filepath.Join(scratch, "outside")
			target := filepath.Join(scratch, "copy")
			if err := os.MkdirAll(outside, 0700); err != nil {
				t.Fatal(err)
			}
			marker := filepath.Join(outside, "marker")
			if err := os.WriteFile(marker, []byte("source unchanged"), 0600); err != nil {
				t.Fatal(err)
			}
			link := filepath.Join(source, filepath.FromSlash(name))
			if err := os.MkdirAll(filepath.Dir(link), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, link); err != nil {
				if runtime.GOOS == "windows" {
					t.Skipf("creating symlinks requires host privileges: %v", err)
				}
				t.Fatal(err)
			}
			if err := copyProduct(source, target); err == nil || !strings.Contains(err.Error(), "symbolic links") {
				t.Fatalf("copy accepted source link: %v", err)
			}
			if _, err := os.Lstat(filepath.Join(target, filepath.FromSlash(name))); !os.IsNotExist(err) {
				t.Fatalf("unsafe link was copied: %v", err)
			}
			data, err := os.ReadFile(marker)
			if err != nil || string(data) != "source unchanged" {
				t.Fatal("source link target changed")
			}
		})
	}
}
