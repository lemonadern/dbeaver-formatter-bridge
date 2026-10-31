// Copyright 2026 lemonadern
// SPDX-License-Identifier: Apache-2.0

// Package p2site prepares immutable flat p2 repositories and composite indexes.
package p2site

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const pluginID = "com.github.lemonadern.dbeaver.formatter.bridge"
const maxSize = 64 << 20

var releaseVersion = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
var repositoryName = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

// ReleaseBase returns the fixed asset base for a plugin tag. Keep the slash
// literal: p2 parses composite child locations with URIUtil.fromString, which
// would escape an existing percent escape again (plugin%2Fv -> plugin%252Fv).
// GitHub accepts literal slashes within the tag in release asset URLs.
func ReleaseBase(repository, version string) (string, error) {
	if !repositoryName.MatchString(repository) || !releaseVersion.MatchString(version) {
		return "", fmt.Errorf("invalid repository or plugin version")
	}
	return "https://github.com/" + repository + "/releases/download/" + "plugin/v" + version + "/", nil
}

// Flatten preserves the input archive and JAR bytes, creating online-only p2
// assets. baseURL may be a loopback HTTP URL in isolated integration tests;
// production callers must supply ReleaseBase and verify the expected inventory.
func Flatten(archive []byte, version, baseURL string) (map[string][]byte, error) {
	if !releaseVersion.MatchString(version) {
		return nil, fmt.Errorf("invalid plugin version")
	}
	if err := validateBase(baseURL); err != nil {
		return nil, err
	}
	files, err := readZIP(archive)
	if err != nil {
		return nil, err
	}
	want := map[string]bool{"p2.index": true, "artifacts.jar": true, "artifacts.xml.xz": true, "content.jar": true, "content.xml.xz": true}
	want["plugins/"+pluginID+"_"+version+".jar"] = true
	want["features/"+pluginID+".feature_"+version+".jar"] = true
	if len(files) != len(want) {
		return nil, fmt.Errorf("unexpected p2 archive inventory")
	}
	for name := range files {
		if !want[name] {
			return nil, fmt.Errorf("unexpected p2 archive entry: %s", name)
		}
	}
	artifactXML, err := metadataXML(files["artifacts.jar"], "artifacts.xml")
	if err != nil {
		return nil, err
	}
	if err = checkArtifacts(artifactXML, files, version); err != nil {
		return nil, err
	}
	contentXML, err := metadataXML(files["content.jar"], "content.xml")
	if err != nil {
		return nil, err
	}
	if err = checkContent(contentXML, version); err != nil {
		return nil, err
	}
	rewritten, err := rewriteMappings(artifactXML, baseURL)
	if err != nil {
		return nil, err
	}
	artifactsJAR, err := metadataJAR("artifacts.xml", rewritten)
	if err != nil {
		return nil, err
	}
	contentJAR, err := metadataJAR("content.xml", contentXML)
	if err != nil {
		return nil, err
	}
	out := map[string][]byte{
		"p2.index":      []byte("version=1\nmetadata.repository.factory.order=content.xml.xz,content.xml,\\!\nartifact.repository.factory.order=artifacts.xml.xz,artifacts.xml,\\!\n"),
		"artifacts.jar": artifactsJAR, "artifacts.xml.xz": encodeXZ(rewritten),
		"content.jar": contentJAR, "content.xml.xz": encodeXZ(contentXML),
	}
	for name, data := range files {
		if strings.HasPrefix(name, "plugins/") || strings.HasPrefix(name, "features/") {
			out[path.Base(name)] = data
		}
	}
	return out, nil
}

func validateBase(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || !strings.HasSuffix(raw, "/") {
		return fmt.Errorf("invalid p2 asset base URL")
	}
	// Our repository names, plugin tag prefixes and numeric versions require
	// no path escapes. Composite readers escape existing percent sequences.
	if strings.Contains(u.EscapedPath(), "%") {
		return fmt.Errorf("p2 asset base URL must use an unescaped path")
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "127.0.0.1" || u.Hostname() == "localhost" || u.Hostname() == "::1")) {
		return fmt.Errorf("p2 asset URL must use HTTPS (loopback HTTP permitted for tests)")
	}
	return nil
}

func readZIP(data []byte) (map[string][]byte, error) {
	if len(data) > maxSize {
		return nil, fmt.Errorf("p2 ZIP exceeds size limit")
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	out := map[string][]byte{}
	total := int64(0)
	if len(zr.File) > 10000 {
		return nil, fmt.Errorf("too many ZIP entries")
	}
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			if path.Clean(strings.TrimSuffix(f.Name, "/")) != strings.TrimSuffix(f.Name, "/") || strings.HasPrefix(f.Name, "/") || strings.Contains(f.Name, "\\") {
				return nil, fmt.Errorf("unexpected ZIP directory")
			}
			continue
		}
		if path.Clean(f.Name) != f.Name || strings.HasPrefix(f.Name, "/") || strings.HasPrefix(f.Name, "../") || f.Name == ".." || strings.Contains(f.Name, "\\") || f.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("unsafe ZIP entry")
		}
		if _, ok := out[f.Name]; ok {
			return nil, fmt.Errorf("duplicate ZIP entry")
		}
		r, e := f.Open()
		if e != nil {
			return nil, e
		}
		b, e := io.ReadAll(io.LimitReader(r, maxSize+1))
		r.Close()
		if e != nil {
			return nil, e
		}
		total += int64(len(b))
		if total > maxSize {
			return nil, fmt.Errorf("expanded p2 ZIP exceeds size limit")
		}
		out[f.Name] = b
	}
	return out, nil
}
func metadataXML(jar []byte, name string) ([]byte, error) {
	files, err := readZIP(jar)
	if err != nil {
		return nil, err
	}
	for key := range files {
		if key != name && key != "META-INF/MANIFEST.MF" {
			return nil, fmt.Errorf("unexpected metadata JAR entry")
		}
	}
	data, ok := files[name]
	if !ok {
		return nil, fmt.Errorf("missing %s", name)
	}
	return data, nil
}
func metadataJAR(name string, data []byte) ([]byte, error) {
	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	w, err := zw.Create(name)
	if err != nil {
		return nil, err
	}
	if _, err = w.Write(data); err != nil {
		return nil, err
	}
	if err = zw.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
func validXML(data []byte) error {
	d := xml.NewDecoder(bytes.NewReader(data))
	for {
		_, err := d.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

type property struct {
	Name  string `xml:"name,attr"`
	Value string `xml:"value,attr"`
}
type artifact struct {
	Classifier string     `xml:"classifier,attr"`
	ID         string     `xml:"id,attr"`
	Version    string     `xml:"version,attr"`
	Properties []property `xml:"properties>property"`
}

func checkArtifacts(data []byte, files map[string][]byte, version string) error {
	var repo struct {
		XMLName   xml.Name   `xml:"repository"`
		Artifacts []artifact `xml:"artifacts>artifact"`
	}
	if err := xml.Unmarshal(data, &repo); err != nil {
		return err
	}
	if len(repo.Artifacts) != 2 {
		return fmt.Errorf("expected exactly two p2 artifacts")
	}
	seen := map[string]bool{}
	for _, a := range repo.Artifacts {
		prefix := ""
		if a.Classifier == "osgi.bundle" && a.ID == pluginID {
			prefix = "plugins/"
		}
		if a.Classifier == "org.eclipse.update.feature" && a.ID == pluginID+".feature" {
			prefix = "features/"
		}
		if prefix == "" || a.Version != version || seen[prefix] {
			return fmt.Errorf("unexpected p2 artifact identity or version")
		}
		seen[prefix] = true
		b := files[prefix+a.ID+"_"+version+".jar"]
		props := map[string]string{}
		for _, p := range a.Properties {
			if _, ok := props[p.Name]; ok {
				return fmt.Errorf("duplicate artifact property")
			}
			props[p.Name] = p.Value
		}
		sum256 := sha256.Sum256(b)
		sum512 := sha512.Sum512(b)
		if props["artifact.size"] != strconv.Itoa(len(b)) || props["download.size"] != strconv.Itoa(len(b)) || props["download.checksum.sha-256"] != hex.EncodeToString(sum256[:]) {
			return fmt.Errorf("p2 artifact size or SHA-256 mismatch: %s", a.ID)
		}
		if val := props["download.checksum.sha-512"]; val != "" && val != hex.EncodeToString(sum512[:]) {
			return fmt.Errorf("p2 artifact SHA-512 mismatch")
		}
	}
	return nil
}
func rewriteMappings(data []byte, base string) ([]byte, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	var b bytes.Buffer
	enc := xml.NewEncoder(&b)
	count := 0
	for {
		t, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if start, ok := t.(xml.StartElement); ok && start.Name.Local == "rule" {
			for i := range start.Attr {
				if start.Attr[i].Name.Local == "output" {
					val := start.Attr[i].Value
					if val == "${repoUrl}/plugins/${id}_${version}.jar" || val == "${repoUrl}/features/${id}_${version}.jar" {
						start.Attr[i].Value = base + "${id}_${version}.jar"
						count++
					} else if val != "${repoUrl}/binary/${id}_${version}" {
						return nil, fmt.Errorf("unexpected p2 mapping rule")
					}
				}
			}
			t = start
		}
		if err := enc.EncodeToken(t); err != nil {
			return nil, err
		}
	}
	if err := enc.Flush(); err != nil {
		return nil, err
	}
	if count != 2 {
		return nil, fmt.Errorf("expected plugin and feature mapping rules")
	}
	return b.Bytes(), nil
}

// Composite emits deterministic uncompressed standard composite metadata.
// Callers must supply only verified, publicly readable plugin Release bases.
func Composite(children []string) (map[string][]byte, error) {
	ordered := append([]string(nil), children...)
	sort.Strings(ordered)
	for i, child := range ordered {
		if err := validateBase(child); err != nil {
			return nil, err
		}
		if i > 0 && ordered[i-1] == child {
			return nil, fmt.Errorf("duplicate composite child")
		}
	}
	create := func(kind, typ string) []byte {
		var b bytes.Buffer
		fmt.Fprintf(&b, "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<?%sRepository version=\"1.0.0\"?>\n<repository name=\"DBeaver Formatter Bridge\" type=\"%s\" version=\"1.0.0\">\n  <children size=\"%d\">\n", kind, typ, len(ordered))
		for _, child := range ordered {
			b.WriteString("    <child location=\"")
			xml.EscapeText(&b, []byte(child))
			b.WriteString("\"/>\n")
		}
		b.WriteString("  </children>\n</repository>\n")
		return b.Bytes()
	}
	return map[string][]byte{
		"p2.index":               []byte("version=1\nmetadata.repository.factory.order=compositeContent.xml,\\!\nartifact.repository.factory.order=compositeArtifacts.xml,\\!\n"),
		"compositeContent.xml":   create("compositeMetadata", "org.eclipse.equinox.internal.p2.metadata.repository.CompositeMetadataRepository"),
		"compositeArtifacts.xml": create("compositeArtifact", "org.eclipse.equinox.internal.p2.artifact.repository.CompositeArtifactRepository"),
	}, nil
}

// checkContent binds installable units and artifact references to the exact
// plugin/feature pair verified by checkArtifacts. Other metadata-only units
// (the category and Java execution environment) may not introduce artifacts.
func checkContent(data []byte, version string) error {
	type capability struct {
		Namespace string `xml:"namespace,attr"`
		Name      string `xml:"name,attr"`
		Version   string `xml:"version,attr"`
		Range     string `xml:"range,attr"`
	}
	type unit struct {
		ID        string       `xml:"id,attr"`
		Version   string       `xml:"version,attr"`
		Artifacts []artifact   `xml:"artifacts>artifact"`
		Provides  []capability `xml:"provides>provided"`
		Requires  []capability `xml:"requires>required"`
	}
	var repo struct {
		XMLName xml.Name `xml:"repository"`
		Units   []unit   `xml:"units>unit"`
	}
	if err := xml.Unmarshal(data, &repo); err != nil {
		return err
	}
	required := map[string]bool{pluginID: false, pluginID + ".feature.feature.jar": false, pluginID + ".feature.feature.group": false}
	seen := map[string]bool{}
	refs := 0
	for _, u := range repo.Units {
		if seen[u.ID] {
			return fmt.Errorf("duplicate p2 content unit: %s", u.ID)
		}
		seen[u.ID] = true
		if _, ok := required[u.ID]; ok {
			if u.Version != version {
				return fmt.Errorf("p2 content unit version mismatch: %s", u.ID)
			}
			required[u.ID] = true
			self := 0
			for _, p := range u.Provides {
				if p.Namespace == "org.eclipse.equinox.p2.iu" && p.Name == u.ID {
					if p.Version != version {
						return fmt.Errorf("p2 content capability version mismatch")
					}
					self++
				}
			}
			if self != 1 {
				return fmt.Errorf("missing p2 self capability: %s", u.ID)
			}
			switch u.ID {
			case pluginID, pluginID + ".feature.feature.jar":
				if len(u.Artifacts) != 1 {
					return fmt.Errorf("expected one p2 content artifact reference")
				}
				a := u.Artifacts[0]
				id, class := pluginID, "osgi.bundle"
				if u.ID != pluginID {
					id += ".feature"
					class = "org.eclipse.update.feature"
				}
				if a.ID != id || a.Classifier != class || a.Version != version {
					return fmt.Errorf("p2 content artifact reference mismatch")
				}
				refs++
			case pluginID + ".feature.feature.group":
				if len(u.Artifacts) != 0 {
					return fmt.Errorf("feature group must not introduce artifacts")
				}
				dependencies := map[string]bool{pluginID: false, pluginID + ".feature.feature.jar": false}
				for _, r := range u.Requires {
					if _, ok := dependencies[r.Name]; ok && r.Namespace == "org.eclipse.equinox.p2.iu" {
						if dependencies[r.Name] || r.Range != "["+version+","+version+"]" {
							return fmt.Errorf("feature group dependency mismatch")
						}
						dependencies[r.Name] = true
					}
				}
				for id, found := range dependencies {
					if !found {
						return fmt.Errorf("feature group missing dependency: %s", id)
					}
				}
			}
		} else if len(u.Artifacts) != 0 {
			return fmt.Errorf("unexpected p2 content artifact owner: %s", u.ID)
		}
	}
	for id, found := range required {
		if !found {
			return fmt.Errorf("missing p2 content unit: %s", id)
		}
	}
	if refs != 2 {
		return fmt.Errorf("expected two p2 content artifact references")
	}
	return nil
}
