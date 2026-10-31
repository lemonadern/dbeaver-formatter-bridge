// Copyright 2026 lemonadern
// SPDX-License-Identifier: Apache-2.0
package p2site

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"os/exec"
	"strings"
	"testing"
)

func zipBytes(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	for name, data := range files {
		w, err := z.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = w.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func fixture(t *testing.T) []byte {
	t.Helper()
	files := map[string][]byte{}
	var artifacts strings.Builder
	for _, entry := range []struct{ prefix, id, class string }{{"plugins/", pluginID, "osgi.bundle"}, {"features/", pluginID + ".feature", "org.eclipse.update.feature"}} {
		data := []byte("JAR data for " + entry.id)
		files[entry.prefix+entry.id+"_0.1.0.jar"] = data
		a := sha256.Sum256(data)
		b := sha512.Sum512(data)
		fmt.Fprintf(&artifacts, `<artifact classifier="%s" id="%s" version="0.1.0"><properties><property name="artifact.size" value="%d"/><property name="download.size" value="%d"/><property name="download.checksum.sha-256" value="%s"/><property name="download.checksum.sha-512" value="%s"/></properties></artifact>`, entry.class, entry.id, len(data), len(data), hex.EncodeToString(a[:]), hex.EncodeToString(b[:]))
	}
	a := []byte(`<repository><mappings><rule output="${repoUrl}/plugins/${id}_${version}.jar"/><rule output="${repoUrl}/features/${id}_${version}.jar"/></mappings><artifacts>` + artifacts.String() + `</artifacts></repository>`)
	files["artifacts.jar"] = zipBytes(t, map[string][]byte{"artifacts.xml": a})
	files["artifacts.xml.xz"] = encodeXZ(a)
	c := contentFixture("0.1.0")
	files["content.jar"] = zipBytes(t, map[string][]byte{"content.xml": c})
	files["content.xml.xz"] = encodeXZ(c)
	files["p2.index"] = []byte("version=1\n")
	return zipBytes(t, files)
}
func TestFlattenPreservesJARsAndPinsMapping(t *testing.T) {
	archive := fixture(t)
	before, err := readZIP(archive)
	if err != nil {
		t.Fatal(err)
	}
	base, err := ReleaseBase("lemonadern/dbeaver-formatter-bridge", "0.1.0")
	if err != nil {
		t.Fatal(err)
	}
	out, err := Flatten(archive, "0.1.0", base)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 7 {
		t.Fatalf("inventory: %v", out)
	}
	for name, data := range before {
		if strings.HasPrefix(name, "plugins/") || strings.HasPrefix(name, "features/") {
			parts := strings.Split(name, "/")
			if !bytes.Equal(data, out[parts[1]]) {
				t.Fatal("JAR changed")
			}
		}
	}
	x, err := metadataXML(out["artifacts.jar"], "artifacts.xml")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(x, []byte("/plugins/")) || bytes.Contains(x, []byte("/features/")) || !bytes.Contains(x, []byte(base)) {
		t.Fatalf("mapping: %s", x)
	}
	for _, name := range []string{"artifacts", "content"} {
		xmlData, err := metadataXML(out[name+".jar"], name+".xml")
		if err != nil {
			t.Fatal(err)
		}
		assertXZ(t, out[name+".xml.xz"], xmlData)
	}
	local, err := Flatten(archive, "0.1.0", "http://127.0.0.1:12345/repository/")
	if err != nil {
		t.Fatal(err)
	}
	localXML, _ := metadataXML(local["artifacts.jar"], "artifacts.xml")
	if !bytes.Equal(bytes.ReplaceAll(x, []byte(base), []byte("BASE/")), bytes.ReplaceAll(localXML, []byte("http://127.0.0.1:12345/repository/"), []byte("BASE/"))) {
		t.Fatal("test and production metadata differ beyond base URL")
	}
	for _, name := range []string{pluginID + "_0.1.0.jar", pluginID + ".feature_0.1.0.jar"} {
		if !bytes.Equal(local[name], out[name]) {
			t.Fatal("test JAR differs")
		}
	}
}
func assertXZ(t *testing.T, encoded, data []byte) {
	t.Helper()
	xz, err := exec.LookPath("xz")
	if err != nil {
		t.Log("xz not installed; external decoder check is covered by integration tests")
		return
	}
	cmd := exec.Command(xz, "-dc")
	cmd.Stdin = bytes.NewReader(encoded)
	decoded, err := cmd.Output()
	if err != nil {
		t.Fatalf("XZ decode: %v", err)
	}
	if !bytes.Equal(decoded, data) {
		t.Fatal("XZ decoded bytes differ")
	}
}
func TestXZBoundarySizes(t *testing.T) {
	for _, size := range []int{0, 1, 127, 128, 65535, 65536, 65537, 131072, 1048576} {
		t.Run(fmt.Sprint(size), func(t *testing.T) { data := bytes.Repeat([]byte("a"), size); assertXZ(t, encodeXZ(data), data) })
	}
}
func TestRejectBrokenInventoryAndHashes(t *testing.T) {
	for _, mutation := range []string{"extra", "bad-hash", "wrong-version", "unsafe", "symlink"} {
		t.Run(mutation, func(t *testing.T) {
			files, _ := readZIP(fixture(t))
			version := "0.1.0"
			switch mutation {
			case "extra":
				files["secret.txt"] = []byte("x")
			case "bad-hash":
				files["plugins/"+pluginID+"_0.1.0.jar"] = []byte("different")
			case "wrong-version":
				version = "0.2.0"
			case "unsafe":
				files["../escape"] = []byte("x")
			case "symlink":
				files["plugins/evil.jar"] = []byte("x")
			}
			if _, err := Flatten(zipBytes(t, files), version, "https://example.org/v0.1.0/"); err == nil {
				t.Fatal("accepted bad archive")
			}
		})
	}
}
func TestCompositeDeterministic(t *testing.T) {
	one, _ := ReleaseBase("owner/repo", "0.1.0")
	two, _ := ReleaseBase("owner/repo", "0.2.0")
	a, err := Composite([]string{two, one})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := Composite([]string{one, two})
	for name, data := range a {
		if !bytes.Equal(data, b[name]) {
			t.Fatal("non-deterministic index")
		}
		if name != "p2.index" {
			if err := validXML(data); err != nil {
				t.Fatal(err)
			}
			var repo struct {
				Children []struct {
					Location string `xml:"location,attr"`
				} `xml:"children>child"`
			}
			if err := xml.Unmarshal(data, &repo); err != nil || len(repo.Children) != 2 {
				t.Fatalf("bad children: %v", err)
			}
		}
	}
	if _, err := Composite([]string{one, one}); err == nil {
		t.Fatal("duplicate accepted")
	}
}
func TestRejectUnsafeBases(t *testing.T) {
	for _, base := range []string{"file:/tmp/", "http://example.org/", "https://user:pass@example.org/", "https://example.org/?q=x", "https://example.org/#frag", "https://example.org", "https://example.org/plugin%2Fv0.1.0/", "https://example.org/plugin%252Fv0.1.0/"} {
		if _, err := Flatten(fixture(t), "0.1.0", base); err == nil {
			t.Fatalf("accepted %q", base)
		}
	}
}

func contentFixture(version string) []byte {
	self := func(id string) string {
		return `<provides><provided namespace="org.eclipse.equinox.p2.iu" name="` + id + `" version="` + version + `"/></provides>`
	}
	return []byte(`<repository><units size="3"><unit id="` + pluginID + `" version="` + version + `">` + self(pluginID) + `<artifacts><artifact classifier="osgi.bundle" id="` + pluginID + `" version="` + version + `"/></artifacts></unit><unit id="` + pluginID + `.feature.feature.jar" version="` + version + `">` + self(pluginID+`.feature.feature.jar`) + `<artifacts><artifact classifier="org.eclipse.update.feature" id="` + pluginID + `.feature" version="` + version + `"/></artifacts></unit><unit id="` + pluginID + `.feature.feature.group" version="` + version + `">` + self(pluginID+`.feature.feature.group`) + `<requires><required namespace="org.eclipse.equinox.p2.iu" name="` + pluginID + `" range="[` + version + `,` + version + `]"/><required namespace="org.eclipse.equinox.p2.iu" name="` + pluginID + `.feature.feature.jar" range="[` + version + `,` + version + `]"/></requires></unit></units></repository>`)
}
func TestRejectMismatchedContent(t *testing.T) {
	for _, name := range []string{"empty", "wrong-unit-version", "wrong-artifact-version", "wrong-artifact-id", "missing-group", "wrong-requirement", "duplicate-unit"} {
		t.Run(name, func(t *testing.T) {
			c := contentFixture("0.1.0")
			switch name {
			case "empty":
				c = []byte(`<repository><units size="0"/></repository>`)
			case "wrong-unit-version":
				c = bytes.Replace(c, []byte(`version="0.1.0"`), []byte(`version="0.2.0"`), 1)
			case "wrong-artifact-version":
				c = bytes.Replace(c, []byte(`id="`+pluginID+`" version="0.1.0"/></artifacts>`), []byte(`id="`+pluginID+`" version="0.2.0"/></artifacts>`), 1)
			case "wrong-artifact-id":
				c = bytes.Replace(c, []byte(`classifier="osgi.bundle" id="`+pluginID+`"`), []byte(`classifier="osgi.bundle" id="other.bundle"`), 1)
			case "missing-group":
				c = bytes.Replace(c, []byte(`id="`+pluginID+`.feature.feature.group"`), []byte(`id="other.group"`), 1)
			case "wrong-requirement":
				c = bytes.Replace(c, []byte(`[0.1.0,0.1.0]`), []byte(`[0.0.0,0.0.0]`), 1)
			case "duplicate-unit":
				c = bytes.Replace(c, []byte(`</units>`), []byte(`<unit id="`+pluginID+`" version="0.1.0"/></units>`), 1)
			}
			files, _ := readZIP(fixture(t))
			files["content.jar"] = zipBytes(t, map[string][]byte{"content.xml": c})
			files["content.xml.xz"] = encodeXZ(c)
			if _, err := Flatten(zipBytes(t, files), "0.1.0", "https://example.org/release/"); err == nil {
				t.Fatal("accepted mismatched content")
			}
		})
	}
}

func TestReleaseBasePreservesTagSlash(t *testing.T) {
	base, err := ReleaseBase("owner/repo", "0.2.0")
	if err != nil {
		t.Fatal(err)
	}
	if base != "https://github.com/owner/repo/releases/download/plugin/v0.2.0/" {
		t.Fatalf("unexpected release base: %s", base)
	}
	for _, encoded := range []string{"https://github.com/owner/repo/releases/download/plugin%2Fv0.2.0/", "https://github.com/owner/repo/releases/download/plugin%252Fv0.2.0/"} {
		if _, err := Composite([]string{encoded}); err == nil {
			t.Fatalf("accepted double-escape-prone composite child: %s", encoded)
		}
	}
}
