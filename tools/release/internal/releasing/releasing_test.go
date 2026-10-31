// Copyright 2026 lemonadern
// SPDX-License-Identifier: Apache-2.0
package releasing

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/lemonadern/dbeaver-formatter-bridge/tools/release/internal/versioning"
)

const testSHA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const testVersion = "0.2.0"
const testRun int64 = 23

var jobNames = []string{"CLI / ubuntu-24.04", "CLI / macos-14", "CLI / windows-2025", "CLI / Package", "VSIX / Package", "VSIX / ubuntu-24.04", "VSIX / macos-14", "VSIX / windows-2025", "Real DBeaver CE 26.2.2 / Windows x64", "Go install / Tagged module", "Release assets / Validate assembly"}

func makeZip(t *testing.T, files map[string][]byte) []byte {
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

type fakeGH struct {
	t              *testing.T
	rel            *release
	sources        map[int64][]byte
	metas          map[int64]artifactMeta
	remote         map[int64][]byte
	data           map[string][]byte
	refs           map[string]artifactRef
	writes         []string
	requests       []string
	jobs           []map[string]any
	failUpload     string
	collectorErr   error
	collectHook    func()
	runSHA         string
	runEvent       string
	nextID         int64
	currentAttempt int
}

func fake(t *testing.T) *fakeGH {
	t.Helper()
	f := &fakeGH{t: t, sources: map[int64][]byte{}, metas: map[int64]artifactMeta{}, remote: map[int64][]byte{}, data: map[string][]byte{}, refs: map[string]artifactRef{}, runSHA: testSHA, runEvent: "push", nextID: 100, currentAttempt: 1}
	i := int64(11)
	for component, prefix := range artifactPrefixes {
		raw := makeZip(t, map[string][]byte{"source.txt": []byte(component)})
		f.sources[i] = raw
		m := artifactMeta{ID: i, Name: prefix + "-23-1", Digest: "sha256:" + digest(raw)}
		m.Workflow.ID = testRun
		m.Workflow.SHA = testSHA
		f.metas[i] = m
		f.refs[component] = artifactRef{ID: i, Name: m.Name, RunID: testRun, Attempt: 1, Digest: m.Digest}
		i++
	}
	for name := range expectedFiles(testVersion) {
		if name != "SHA256SUMS" {
			f.data[name] = []byte("distribution: " + name)
		}
	}
	var sum strings.Builder
	for name, data := range f.data {
		fmt.Fprintf(&sum, "%s  %s\n", digest(data), name)
	}
	f.data["SHA256SUMS"] = []byte(sum.String())
	for i, name := range jobNames {
		f.jobs = append(f.jobs, map[string]any{"id": i + 1, "name": "Build and test / " + name, "status": "completed", "conclusion": "success", "run_attempt": 1})
	}
	return f
}
func (f *fakeGH) provenance() record {
	r := record{Schema: 1, Version: testVersion, Commit: testSHA, RunID: testRun, Attempt: 1, Artifacts: f.refs, Files: map[string]string{}}
	for name, data := range f.data {
		r.Files[name] = digest(data)
	}
	return r
}
func (f *fakeGH) existing(draft bool, count int) {
	body, err := notes("owner/repo", f.provenance())
	if err != nil {
		f.t.Fatal(err)
	}
	f.rel = &release{ID: 1, Tag: "v" + testVersion, Draft: draft, Body: body}
	i := 0
	for name, data := range f.data {
		if i >= count {
			break
		}
		f.attach(name, data)
		i++
	}
}
func (f *fakeGH) attach(name string, data []byte) {
	f.nextID++
	f.remote[f.nextID] = append([]byte(nil), data...)
	f.rel.Assets = append(f.rel.Assets, asset{ID: f.nextID, Name: name, Size: int64(len(data))})
}
func (f *fakeGH) execute(args []string) ([]byte, error) {
	f.requests = append(f.requests, strings.Join(args, " "))
	if len(args) > 1 && args[0] == "api" && args[1] == "--help" {
		return []byte("--allow-escape-sequences"), nil
	}
	if args[0] == "api" {
		endpoint := ""
		for _, arg := range args {
			if strings.HasPrefix(arg, "repos/") {
				endpoint = strings.TrimPrefix(arg, "repos/owner/repo/")
			}
		}
		var response any
		switch {
		case strings.HasPrefix(endpoint, "git/ref/tags/"):
			response = map[string]any{"object": map[string]string{"type": "tag", "sha": strings.Repeat("b", 40)}}
		case strings.HasPrefix(endpoint, "git/tags/"):
			response = map[string]any{"object": map[string]string{"type": "commit", "sha": testSHA}}
		case strings.HasPrefix(endpoint, "releases?"):
			if f.rel == nil {
				response = []release{}
			} else {
				response = []release{*f.rel}
			}
		case strings.HasPrefix(endpoint, "releases/assets/"):
			id, _ := strconv.ParseInt(strings.TrimPrefix(endpoint, "releases/assets/"), 10, 64)
			data, ok := f.remote[id]
			if !ok {
				return nil, fmt.Errorf("missing remote asset")
			}
			return data, nil
		case strings.HasPrefix(endpoint, "actions/artifacts/"):
			value := strings.TrimPrefix(endpoint, "actions/artifacts/")
			id, _ := strconv.ParseInt(strings.TrimSuffix(value, "/zip"), 10, 64)
			if strings.HasSuffix(value, "/zip") {
				data, ok := f.sources[id]
				if !ok {
					return nil, fmt.Errorf("missing artifact")
				}
				return data, nil
			}
			m, ok := f.metas[id]
			if !ok {
				return nil, fmt.Errorf("missing artifact metadata")
			}
			response = m
		case endpoint == "actions/runs/23":
			response = map[string]any{"head_sha": f.runSHA, "event": f.runEvent, "head_branch": "v" + testVersion, "status": "in_progress", "conclusion": nil, "path": ".github/workflows/release.yml", "run_attempt": f.currentAttempt}
		case strings.HasPrefix(endpoint, "actions/runs/23/jobs?"):
			response = map[string]any{"jobs": f.jobs}
		default:
			return nil, fmt.Errorf("unexpected API endpoint: %s", endpoint)
		}
		return json.Marshal(response)
	}
	if args[0] != "release" {
		return nil, fmt.Errorf("unexpected command")
	}
	f.writes = append(f.writes, args[1])
	switch args[1] {
	case "create":
		if f.rel != nil {
			return nil, fmt.Errorf("release already exists")
		}
		var file string
		for i, arg := range args {
			if arg == "--notes-file" {
				file = args[i+1]
			}
		}
		data, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		f.rel = &release{ID: 1, Tag: args[2], Draft: true, Body: string(data)}
	case "upload":
		name := filepath.Base(args[3])
		if name == f.failUpload {
			return nil, fmt.Errorf("simulated interrupted upload")
		}
		for _, a := range f.rel.Assets {
			if a.Name == name {
				return nil, fmt.Errorf("upload attempted to replace an existing asset")
			}
		}
		data, err := os.ReadFile(args[3])
		if err != nil {
			return nil, err
		}
		f.attach(name, data)
	case "edit":
		f.rel.Draft = false
	default:
		return nil, fmt.Errorf("unexpected release mutation")
	}
	return nil, nil
}
func (f *fakeGH) engine() *engine {
	e := &engine{root: f.t.TempDir(), version: testVersion, sha: testSHA, tag: "v" + testVersion, client: newClient("owner/repo", f.execute)}
	e.collector = func(input, out string) (map[string]string, error) {
		if f.collectorErr != nil {
			return nil, f.collectorErr
		}
		if f.collectHook != nil {
			f.collectHook()
		}
		if err := os.MkdirAll(out, 0755); err != nil {
			return nil, err
		}
		hashes := map[string]string{}
		for name, data := range f.data {
			if err := os.WriteFile(filepath.Join(out, name), data, 0644); err != nil {
				return nil, err
			}
			hashes[name] = digest(data)
		}
		return hashes, nil
	}
	return e
}
func (f *fakeGH) ids() map[string]int64 {
	m := map[string]int64{}
	for component, ref := range f.refs {
		m[component] = ref.ID
	}
	return m
}
func noWrites(t *testing.T, f *fakeGH) {
	t.Helper()
	if len(f.writes) != 0 {
		t.Fatalf("unexpected writes: %v", f.writes)
	}
}
func TestNewReleaseRecordsBeforeUploadAndVerifiesBeforePublish(t *testing.T) {
	f := fake(t)
	e := f.engine()
	if err := e.publish("", testRun, 1, f.ids(), false); err != nil {
		t.Fatal(err)
	}
	if f.rel.Draft || len(f.rel.Assets) != 9 {
		t.Fatal("release not complete")
	}
	if f.writes[0] != "create" || f.writes[len(f.writes)-1] != "edit" {
		t.Fatal(f.writes)
	}
	r, err := parseRecord(f.rel.Body, testVersion, testSHA)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Files) != 9 || len(r.Artifacts) != 3 {
		t.Fatal("missing provenance")
	}
}
func TestDryRunNeverWrites(t *testing.T) {
	f := fake(t)
	if err := f.engine().publish("", testRun, 1, f.ids(), true); err != nil {
		t.Fatal(err)
	}
	noWrites(t, f)
}
func TestValidationFailuresDoNotCreateRelease(t *testing.T) {
	cases := map[string]func(*fakeGH){
		"job-failed":  func(f *fakeGH) { f.jobs[0]["conclusion"] = "failure" },
		"job-missing": func(f *fakeGH) { f.jobs = f.jobs[:len(f.jobs)-1] },
		"latest-rerun-failed": func(f *fakeGH) {
			f.jobs = append(f.jobs, map[string]any{"id": 999, "name": f.jobs[0]["name"], "status": "completed", "conclusion": "failure", "run_attempt": 1})
		},
		"wrong-run-sha": func(f *fakeGH) { f.runSHA = strings.Repeat("b", 40) },
		"pull-request":  func(f *fakeGH) { f.runEvent = "pull_request" },
		"expired": func(f *fakeGH) {
			for id, m := range f.metas {
				m.Expired = true
				f.metas[id] = m
				break
			}
		},
		"wrong-producer-attempt": func(f *fakeGH) {
			for id, m := range f.metas {
				m.Name = strings.Replace(m.Name, "-23-1", "-23-9", 1)
				f.metas[id] = m
				break
			}
		},
		"artifact-digest": func(f *fakeGH) {
			for id := range f.sources {
				f.sources[id] = []byte("changed archive")
				break
			}
		},
		"collector-failed": func(f *fakeGH) { f.collectorErr = fmt.Errorf("invalid distribution") },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			f := fake(t)
			change(f)
			if err := f.engine().publish("", testRun, 1, f.ids(), false); err == nil {
				t.Fatal("accepted invalid release")
			}
			noWrites(t, f)
		})
	}
}
func TestPublishedReleaseIsNoOpWithoutSourceArtifacts(t *testing.T) {
	f := fake(t)
	f.existing(false, 9)
	f.metas = nil
	f.sources = nil
	if err := f.engine().publish("missing", 0, 0, nil, false); err != nil {
		t.Fatal(err)
	}
	noWrites(t, f)
}
func TestPublishedCorruptionAndLegacyStop(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		t.Run(strconv.FormatBool(legacy), func(t *testing.T) {
			f := fake(t)
			f.existing(false, 9)
			if legacy {
				f.rel.Body = "hand-written release"
			} else {
				f.remote[f.rel.Assets[0].ID][0] ^= 1
			}
			if err := f.engine().publish("", 0, 0, nil, false); err == nil {
				t.Fatal("accepted corrupt/legacy release")
			}
			noWrites(t, f)
		})
	}
}
func TestDraftRestoresOriginalArtifactsAndOnlyMissingFiles(t *testing.T) {
	f := fake(t)
	f.existing(true, 2)
	if err := f.engine().publish("missing-current-attempt", 999, 9, map[string]int64{"cli": 999}, false); err != nil {
		t.Fatal(err)
	}
	if len(f.rel.Assets) != 9 || f.rel.Draft {
		t.Fatal("did not resume")
	}
	uploads := 0
	for _, op := range f.writes {
		if op == "create" {
			t.Fatal("recreated draft")
		}
		if op == "upload" {
			uploads++
		}
	}
	if uploads != 7 {
		t.Fatal(uploads)
	}
}
func TestInterruptedUploadCanResumeWithoutRebuilding(t *testing.T) {
	f := fake(t)
	for name := range f.data {
		f.failUpload = name
		break
	}
	e := f.engine()
	if err := e.publish("", testRun, 1, f.ids(), false); err == nil {
		t.Fatal("expected upload failure")
	}
	if f.rel == nil || !f.rel.Draft {
		t.Fatal("must remain draft")
	}
	f.failUpload = ""
	if err := e.publish("", 0, 0, nil, false); err != nil {
		t.Fatal(err)
	}
	creates := 0
	for _, op := range f.writes {
		if op == "create" {
			creates++
		}
	}
	if creates != 1 || f.rel.Draft || len(f.rel.Assets) != 9 {
		t.Fatal("invalid resume")
	}
}
func TestDraftMissingOriginalArtifactStops(t *testing.T) {
	f := fake(t)
	f.existing(true, 2)
	for id, m := range f.metas {
		m.Expired = true
		f.metas[id] = m
		break
	}
	if err := f.engine().publish("", 0, 0, nil, false); err == nil {
		t.Fatal("accepted expired original")
	}
	noWrites(t, f)
}
func TestPublisherRechecksStateAfterOldPreflight(t *testing.T) {
	f := fake(t)
	e := f.engine()
	mode, err := e.preflight()
	if err != nil || mode != "new" {
		t.Fatal(mode, err)
	}
	f.existing(false, 9)
	if err := e.publish("", 0, 0, nil, false); err != nil {
		t.Fatal(err)
	}
	noWrites(t, f)
}
func TestDraftChangedDuringStagingStopsBeforeUpload(t *testing.T) {
	f := fake(t)
	f.existing(true, 1)
	f.collectHook = func() { f.rel.Body = "changed externally" }
	if err := f.engine().publish("", 0, 0, nil, false); err == nil {
		t.Fatal("accepted changed draft")
	}
	noWrites(t, f)
}
func TestProvenanceStrictness(t *testing.T) {
	f := fake(t)
	body, err := notes("owner/repo", f.provenance())
	if err != nil {
		t.Fatal(err)
	}
	cases := []string{"legacy", body + body, strings.Replace(body, `"schema": 1`, `"schema": 2`, 1), strings.Replace(body, `"schema": 1`, `"extra": true, "schema": 1`, 1), strings.ReplaceAll(body, testSHA, strings.Repeat("b", 40)), strings.Replace(body, markerEnd, "", 1)}
	for _, text := range cases {
		if _, err := parseRecord(text, testVersion, testSHA); err == nil {
			t.Fatal("accepted invalid provenance")
		}
	}
}
func TestSafeArchiveExtraction(t *testing.T) {
	for _, name := range []string{"../escape", "/absolute", "a/../escape", `a\b`} {
		if _, err := zipContents(makeZip(t, map[string][]byte{name: []byte("x")}), 1024); err == nil {
			t.Fatalf("accepted %q", name)
		}
	}
	raw := makeZip(t, map[string][]byte{"big": bytes.Repeat([]byte("x"), 1025)})
	if _, err := zipContents(raw, 1024); err == nil {
		t.Fatal("accepted oversized zip")
	}
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	for i := 0; i < 2; i++ {
		f, _ := w.Create("duplicate")
		_, _ = f.Write([]byte("x"))
	}
	_ = w.Close()
	if _, err := zipContents(b.Bytes(), 1024); err == nil {
		t.Fatal("accepted duplicate")
	}
}
func TestChecksumInventoryRejectsMissingDuplicateAndUnsafeNames(t *testing.T) {
	allowed := map[string]bool{"one.zip": true}
	sum := strings.Repeat("a", 64) + "  one.zip\n"
	if _, err := parseSums([]byte(sum), allowed); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{"", sum + sum, strings.Replace(sum, "one.zip", "../escape", 1), "not-a-hash  one.zip\n"} {
		if _, err := parseSums([]byte(raw), allowed); err == nil {
			t.Fatal("accepted checksum")
		}
	}
}
func definitionFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	source, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"VERSION", "plugin/pom.xml", "plugin/bundles/formatter-bridge/pom.xml", "plugin/features/formatter-bridge/pom.xml", "plugin/repository/pom.xml", "plugin/bundles/formatter-bridge/META-INF/MANIFEST.MF", "plugin/features/formatter-bridge/feature.xml", "extensions/vscode/package.json", "extensions/vscode/package-lock.json", "cli/internal/version/value.go"} {
		data, err := os.ReadFile(filepath.Join(source, name))
		// Legacy API tests keep a synthetic root VERSION; the real checkout now
		// has independent component version files.
		if name == "VERSION" {
			data, err = []byte("0.1.0\n"), nil
		}
		if err != nil {
			t.Fatal(err)
		}
		dest := filepath.Join(root, name)
		if err = os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(dest, data, 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := versioning.Prepare(root, testVersion); err != nil {
		t.Fatal(err)
	}
	cmds := [][]string{{"init", "-q", "-b", "main"}, {"config", "user.name", "Release Test"}, {"config", "user.email", "test@example.invalid"}, {"config", "core.autocrlf", "false"}, {"add", "."}, {"commit", "-qm", "fixture"}, {"tag", "-a", "v" + testVersion, "-m", "fixture"}, {"tag", "-a", "cli/v" + testVersion, "-m", "fixture"}}
	for _, args := range cmds {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s %v", args, output, err)
		}
	}
	return root
}
func TestTagValidationBeforeGitHubAccess(t *testing.T) {
	root := definitionFixture(t)
	if _, _, err := resolve(root, "v"+testVersion); err != nil {
		t.Fatal(err)
	}
	for _, tag := range []string{testVersion, "v0.2.0-rc.1", "v0.3.0"} {
		if _, _, err := resolve(root, tag); err == nil {
			t.Fatalf("accepted %s", tag)
		}
	}
	cmd := exec.Command("git", "tag", "-d", "cli/v"+testVersion)
	cmd.Dir = root
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := resolve(root, "v"+testVersion); err == nil {
		t.Fatal("accepted missing CLI tag")
	}
}

func TestResumeUsesTheOriginalValidatedAttemptCohort(t *testing.T) {
	f := fake(t)
	f.existing(true, 1)
	f.currentAttempt = 2
	// rerun-all can skip build jobs because a draft already exists. That must not
	// hide, or replace, the successful jobs that produced the recorded artifacts.
	for i, name := range jobNames {
		f.jobs = append(f.jobs, map[string]any{"id": 1000 + i, "name": "Build and test / " + name, "status": "completed", "conclusion": "skipped", "run_attempt": 2})
	}
	if err := f.engine().publish("", 0, 0, nil, false); err != nil {
		t.Fatal(err)
	}
	if f.rel.Draft || len(f.rel.Assets) != 9 {
		t.Fatal("original cohort did not resume")
	}
}

func TestPublicationRejectsChangedArtifactProvenanceEvenWhenFileHashesMatch(t *testing.T) {
	for _, change := range []string{"artifact-id", "producer-attempt", "record-attempt"} {
		t.Run(change, func(t *testing.T) {
			f := fake(t)
			f.existing(true, 1)
			f.currentAttempt = 2
			initial := f.provenance()
			initial.Attempt = 2
			originalBody, err := notes("owner/repo", initial)
			if err != nil {
				t.Fatal(err)
			}
			f.rel.Body = originalBody
			f.collectHook = func() {
				current, err := parseRecord(f.rel.Body, testVersion, testSHA)
				if err != nil {
					t.Fatal(err)
				}
				switch change {
				case "artifact-id":
					ref := current.Artifacts["cli"]
					ref.ID += 1000
					current.Artifacts["cli"] = ref
				case "producer-attempt":
					ref := current.Artifacts["cli"]
					ref.Attempt = 2
					ref.Name = "cli-distributions-23-2"
					current.Artifacts["cli"] = ref
				case "record-attempt":
					current.Attempt = 1
				}
				body, err := notes("owner/repo", current)
				if err != nil {
					t.Fatal(err)
				}
				f.rel.Body = body
			}
			if err := f.engine().publish("", 0, 0, nil, false); err == nil {
				t.Fatal("accepted modified source provenance")
			}
			noWrites(t, f)
		})
	}
}

func setProducerAttempt(f *fakeGH, component string, attempt int) {
	ref := f.refs[component]
	ref.Attempt = attempt
	ref.Name = artifactPrefixes[component] + "-23-" + strconv.Itoa(attempt)
	f.refs[component] = ref
	metadata := f.metas[ref.ID]
	metadata.Name = ref.Name
	f.metas[ref.ID] = metadata
}
func setJobAttempt(f *fakeGH, name string, attempt int) {
	for _, job := range f.jobs {
		if job["name"] == "Build and test / "+name {
			job["run_attempt"] = attempt
		}
	}
}
func TestPublicationBindsTestAttemptsToTheRecordedProducers(t *testing.T) {
	cases := map[string]func(*fakeGH){
		"producer-job-is-old":  func(f *fakeGH) { setProducerAttempt(f, "vscode", 2) },
		"vscode-tests-are-old": func(f *fakeGH) { setProducerAttempt(f, "vscode", 2); setJobAttempt(f, "VSIX / Package", 2) },
		"real-dbeaver-tested-old-vsix": func(f *fakeGH) {
			setProducerAttempt(f, "vscode", 2)
			setJobAttempt(f, "VSIX / Package", 2)
			for _, system := range []string{"ubuntu-24.04", "macos-14", "windows-2025"} {
				setJobAttempt(f, "VSIX / "+system, 2)
			}
		},
		"assembly-tested-old-cli": func(f *fakeGH) { setProducerAttempt(f, "cli", 2); setJobAttempt(f, "CLI / Package", 2) },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			f := fake(t)
			f.currentAttempt = 2
			change(f)
			if err := f.engine().publish("", testRun, 2, f.ids(), false); err == nil {
				t.Fatal("accepted tests predating source artifacts")
			}
			noWrites(t, f)
		})
	}
}
func TestPublicationAcceptsMixedProducerAttemptsAfterFailedJobReruns(t *testing.T) {
	f := fake(t)
	f.currentAttempt = 3
	setProducerAttempt(f, "cli", 2)
	setJobAttempt(f, "CLI / Package", 2)
	setProducerAttempt(f, "vscode", 2)
	setJobAttempt(f, "VSIX / Package", 2)
	setProducerAttempt(f, "plugin", 3)
	setJobAttempt(f, "Real DBeaver CE 26.2.2 / Windows x64", 3)
	for _, system := range []string{"ubuntu-24.04", "macos-14", "windows-2025"} {
		setJobAttempt(f, "VSIX / "+system, 2)
	}
	setJobAttempt(f, "Release assets / Validate assembly", 3)
	if err := f.engine().publish("", testRun, 3, f.ids(), false); err != nil {
		t.Fatal(err)
	}
	if f.rel.Draft || len(f.rel.Assets) != 9 {
		t.Fatal("mixed producer recovery did not publish")
	}
}
