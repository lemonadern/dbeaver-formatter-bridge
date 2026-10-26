// Copyright 2026 lemonadern
// SPDX-License-Identifier: Apache-2.0
package releasing

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/lemonadern/dbeaver-formatter-bridge/tools/release/internal/versioning"
)

var hashRE = regexp.MustCompile(`^[a-f0-9]{64}$`)
var shaRE = regexp.MustCompile(`^[a-f0-9]{40}$`)
var repoRE = regexp.MustCompile(`^[A-Za-z0-9_-]+/[A-Za-z0-9_.-]+$`)

const markerStart = "<!-- dbeaver-release-provenance\n"
const markerEnd = "\n-->"

type record struct {
	Schema    int                    `json:"schema"`
	Version   string                 `json:"version"`
	Commit    string                 `json:"commit"`
	RunID     int64                  `json:"run_id"`
	Attempt   int                    `json:"attempt"`
	Artifacts map[string]artifactRef `json:"artifacts"`
	Files     map[string]string      `json:"files"`
}

func (r record) validate(version, sha string) error {
	if r.Schema != 1 || r.Version != version || r.Commit != sha || r.RunID < 1 || r.Attempt < 1 {
		return fmt.Errorf("release provenance does not match version and commit")
	}
	expected := expectedFiles(version)
	if len(r.Files) != len(expected) || len(r.Artifacts) != 3 {
		return fmt.Errorf("release provenance inventory mismatch")
	}
	for name, hash := range r.Files {
		if !expected[name] || !hashRE.MatchString(hash) {
			return fmt.Errorf("invalid release provenance hash")
		}
	}
	ids := map[int64]bool{}
	for component, prefix := range artifactPrefixes {
		a, ok := r.Artifacts[component]
		if !ok || a.ID < 1 || ids[a.ID] || a.RunID != r.RunID || a.Attempt < 1 || a.Attempt > r.Attempt || a.Name != prefix+"-"+strconv.FormatInt(r.RunID, 10)+"-"+strconv.Itoa(a.Attempt) || !strings.HasPrefix(a.Digest, "sha256:") || !hashRE.MatchString(strings.TrimPrefix(a.Digest, "sha256:")) {
			return fmt.Errorf("invalid source artifact provenance")
		}
		ids[a.ID] = true
	}
	return nil
}
func parseRecord(body, version, sha string) (record, error) {
	var r record
	if strings.Count(body, markerStart) != 1 {
		return r, fmt.Errorf("release has no unique automation provenance; legacy releases are read-only")
	}
	_, rest, _ := strings.Cut(body, markerStart)
	raw, _, ok := strings.Cut(rest, markerEnd)
	if !ok {
		return r, fmt.Errorf("unterminated release provenance")
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&r); err != nil {
		return r, fmt.Errorf("invalid release provenance: %w", err)
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF {
		return r, fmt.Errorf("trailing release provenance data")
	}
	if err := r.validate(version, sha); err != nil {
		return r, err
	}
	return r, nil
}
func notes(repo string, r record) (string, error) {
	body, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", err
	}
	text := fmt.Sprintf("## DBeaver Formatter Bridge v%s\n\nFormat SQL through the formatter in your running DBeaver instance.\n\n## Installation\n\n- Install the versioned p2 ZIP in DBeaver (Help → Install New Software → Add → Archive), then restart DBeaver.\n- Extract the CLI ZIP for your OS and CPU and put dbeaver-format on PATH. The Go alternative is:\n\n```sh\ngo install github.com/%s/cli/cmd/dbeaver-format@v%s\n```\n\n- Install the VSIX in desktop VS Code; the CLI is installed separately. Configure dbeaverFormatter.workspace and choose DBeaver Formatter Bridge as the SQL formatter.\n- Verify the distribution files with SHA256SUMS.\n\n[Installation](https://github.com/%s/blob/v%s/docs/installation.md) / [Compatibility and test scope](https://github.com/%s/blob/v%s/docs/compatibility.md)\n\nDBeaver must be running with the bridge plugin; selected connections must already be connected. See compatibility for tested DBeaver/VS Code versions, CPU coverage, test doubles and known limitations. Marketplace, Web and Remote extension distribution are not part of this release.\n\n## Verification\n\nCommit: `%s`\n\n[Successful build and test jobs](https://github.com/%s/actions/runs/%d). These are the tested artifacts, not new builds from the publication job.\n\n## License\n\nApache-2.0. Copyright 2026 lemonadern. DBeaver itself is not redistributed.\n\n", r.Version, repo, r.Version, repo, r.Version, repo, r.Version, r.Commit, repo, r.RunID)
	return text + markerStart + string(body) + markerEnd + "\n", nil
}
func git(root string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	data, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %s failed", strings.Join(args, " "))
	}
	return strings.TrimSpace(string(data)), nil
}
func resolve(root, tag string) (string, string, error) {
	version := strings.TrimPrefix(tag, "v")
	if tag != "v"+version {
		return "", "", fmt.Errorf("product tag must start with v")
	}
	if err := versioning.Validate(version); err != nil {
		return "", "", err
	}
	actual, err := versioning.Check(root)
	if err != nil {
		return "", "", err
	}
	if actual != version {
		return "", "", fmt.Errorf("tag and source VERSION do not match")
	}
	head, err := git(root, "rev-parse", "HEAD")
	if err != nil {
		return "", "", err
	}
	if !shaRE.MatchString(head) {
		return "", "", fmt.Errorf("invalid Git commit")
	}
	for _, ref := range []string{"refs/tags/" + tag + "^{}", "refs/tags/cli/" + tag + "^{}"} {
		sha, err := git(root, "rev-parse", ref)
		if err != nil || sha != head {
			return "", "", fmt.Errorf("both release tags must point to HEAD")
		}
	}
	for _, args := range [][]string{{"diff", "--quiet"}, {"diff", "--cached", "--quiet"}} {
		if _, err := git(root, args...); err != nil {
			return "", "", fmt.Errorf("release source has tracked changes")
		}
	}
	return version, head, nil
}

type engine struct {
	root, version, sha, tag string
	client                  *client
	collector               func(string, string) (map[string]string, error)
}

func (e *engine) verifyTags() error {
	type object struct {
		Type string `json:"type"`
		SHA  string `json:"sha"`
	}
	for _, tag := range []string{e.tag, "cli/" + e.tag} {
		var ref struct {
			Object object `json:"object"`
		}
		if err := e.client.api("git/ref/tags/"+tag, &ref); err != nil {
			return err
		}
		if ref.Object.Type != "tag" {
			return fmt.Errorf("release tags must be annotated")
		}
		target := ref.Object
		for depth := 0; target.Type == "tag"; depth++ {
			if depth >= 8 || !shaRE.MatchString(target.SHA) {
				return fmt.Errorf("invalid annotated tag chain")
			}
			var annotated struct {
				Object object `json:"object"`
			}
			if err := e.client.api("git/tags/"+target.SHA, &annotated); err != nil {
				return err
			}
			target = annotated.Object
		}
		if target.Type != "commit" || target.SHA != e.sha {
			return fmt.Errorf("remote release tags changed or disagree with source commit")
		}
	}
	return nil
}

func (e *engine) inspect() (*release, *record, error) {
	if err := e.verifyTags(); err != nil {
		return nil, nil, err
	}
	rel, err := e.client.findRelease(e.tag)
	if err != nil || rel == nil {
		return rel, nil, err
	}
	r, err := parseRecord(rel.Body, e.version, e.sha)
	if err != nil {
		return nil, nil, err
	}
	return rel, &r, nil
}
func (e *engine) verifyRemote(rel *release, r record, complete bool) error {
	names := map[string]bool{}
	for _, a := range rel.Assets {
		expected, ok := r.Files[a.Name]
		if !ok || names[a.Name] {
			return fmt.Errorf("unexpected or duplicate uploaded asset: %s", a.Name)
		}
		names[a.Name] = true
		data, err := e.client.assetData(a)
		if err != nil {
			return err
		}
		if digest(data) != expected {
			return fmt.Errorf("uploaded asset checksum mismatch: %s", a.Name)
		}
	}
	if complete && len(names) != len(r.Files) {
		return fmt.Errorf("release asset inventory is incomplete")
	}
	return nil
}
func (e *engine) preflight() (string, error) {
	rel, r, err := e.inspect()
	if err != nil {
		return "", err
	}
	if rel == nil {
		return "new", nil
	}
	if !rel.Draft {
		if err = e.verifyRemote(rel, *r, true); err != nil {
			return "", err
		}
		return "done", nil
	}
	if err = e.verifyRemote(rel, *r, false); err != nil {
		return "", err
	}
	for component, ref := range r.Artifacts {
		actual, err := e.client.artifact(ref.ID, r.RunID, e.sha, component)
		if err != nil {
			return "", err
		}
		if actual != ref {
			return "", fmt.Errorf("original draft artifact changed")
		}
	}
	return "resume", nil
}
func (e *engine) downloadArtifacts(refs map[string]artifactRef, runID int64, dest string) error {
	for component, ref := range refs {
		actual, err := e.client.artifact(ref.ID, runID, e.sha, component)
		if err != nil {
			return err
		}
		if actual != ref {
			return fmt.Errorf("original artifact metadata changed")
		}
		raw, err := e.client.binary("actions/artifacts/"+strconv.FormatInt(ref.ID, 10)+"/zip", "application/vnd.github+json")
		if err != nil {
			return err
		}
		if "sha256:"+digest(raw) != ref.Digest {
			return fmt.Errorf("source artifact digest mismatch")
		}
		entries, err := zipContents(raw, 256<<20)
		if err != nil {
			return err
		}
		target := filepath.Join(dest, component)
		if err = os.MkdirAll(target, 0755); err != nil {
			return err
		}
		for name, data := range entries {
			// Uploaded distributions are flat; do not accept alternate extraction roots.
			if pathBase(name) != name {
				return fmt.Errorf("unexpected nested distribution artifact")
			}
			file := filepath.Join(target, name)
			f, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
			if err != nil {
				return err
			}
			_, err = f.Write(data)
			closeErr := f.Close()
			if err != nil {
				return err
			}
			if closeErr != nil {
				return closeErr
			}
		}
	}
	return nil
}
func pathBase(name string) string {
	return strings.TrimPrefix(filepath.ToSlash(filepath.Base(filepath.FromSlash(name))), "./")
}
func compareInputs(supplied, downloaded string) error {
	if supplied == "" {
		return nil
	}
	for component := range artifactPrefixes {
		left, err := os.ReadDir(filepath.Join(supplied, component))
		if err != nil {
			return err
		}
		right, err := os.ReadDir(filepath.Join(downloaded, component))
		if err != nil {
			return err
		}
		if len(left) != len(right) {
			return fmt.Errorf("provided artifact inventory mismatch")
		}
		for _, f := range right {
			a, err := readRegular(filepath.Join(supplied, component, f.Name()), 128<<20)
			if err != nil {
				return err
			}
			b, err := readRegular(filepath.Join(downloaded, component, f.Name()), 128<<20)
			if err != nil {
				return err
			}
			if !bytes.Equal(a, b) {
				return fmt.Errorf("provided artifact does not match exact source artifact ID")
			}
		}
	}
	return nil
}
func (e *engine) stage(input, out string) (map[string]string, error) {
	if e.collector != nil {
		return e.collector(input, out)
	}
	return collect(e.root, e.version, e.sha, input, out)
}
func equalHashes(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for name, hash := range a {
		if b[name] != hash {
			return false
		}
	}
	return true
}
func (e *engine) publish(input string, runID int64, attempt int, ids map[string]int64, dryRun bool) error {
	// Re-read even after successful preflight: rerun-failed may skip that old job.
	rel, existing, err := e.inspect()
	if err != nil {
		return err
	}
	if rel != nil && !rel.Draft {
		return e.verifyRemote(rel, *existing, true)
	}
	var r record
	if existing != nil {
		r = *existing
		if err = e.verifyRemote(rel, r, false); err != nil {
			return err
		}
	} else {
		if runID < 1 || attempt < 1 {
			return fmt.Errorf("positive publication run and attempt required")
		}
		r = record{Schema: 1, Version: e.version, Commit: e.sha, RunID: runID, Attempt: attempt, Artifacts: map[string]artifactRef{}}
		for component := range artifactPrefixes {
			ref, err := e.client.artifact(ids[component], runID, e.sha, component)
			if err != nil {
				return err
			}
			if ref.Attempt > attempt {
				return fmt.Errorf("artifact producer is newer than publisher attempt")
			}
			r.Artifacts[component] = ref
		}
	}
	if err = e.client.verifyRun(r.RunID, e.sha, e.tag, r.Attempt, r.Artifacts); err != nil {
		return err
	}
	temp, err := os.MkdirTemp("", "dbeaver-release-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temp)
	downloaded := filepath.Join(temp, "input")
	if err = e.downloadArtifacts(r.Artifacts, r.RunID, downloaded); err != nil {
		return err
	}
	// On resume, original recorded IDs override current attempt's supplied inputs.
	if existing == nil {
		if err = compareInputs(input, downloaded); err != nil {
			return err
		}
	}
	stage := filepath.Join(temp, "assets")
	hashes, err := e.stage(downloaded, stage)
	if err != nil {
		return err
	}
	if existing != nil && !equalHashes(hashes, r.Files) {
		return fmt.Errorf("restored artifacts differ from draft provenance")
	}
	r.Files = hashes
	if err = r.validate(e.version, e.sha); err != nil {
		return err
	}
	if dryRun {
		fmt.Println("PASS: release staging verified; no release writes performed")
		return nil
	}
	if rel == nil {
		body, err := notes(e.client.repo, r)
		if err != nil {
			return err
		}
		notesFile := filepath.Join(temp, "notes.md")
		if err = os.WriteFile(notesFile, []byte(body), 0600); err != nil {
			return err
		}
		if _, err = e.client.run([]string{"release", "create", e.tag, "--repo", e.client.repo, "--draft", "--verify-tag", "--target", e.sha, "--title", e.tag, "--notes-file", notesFile}); err != nil {
			return err
		}
	}
	// Another publisher or manual action may have changed state during staging.
	rel, check, err := e.inspect()
	if err != nil {
		return err
	}
	if rel == nil {
		return fmt.Errorf("release disappeared before upload")
	}
	if !reflect.DeepEqual(*check, r) {
		return fmt.Errorf("release provenance changed before upload")
	}
	if !rel.Draft {
		return e.verifyRemote(rel, *check, true)
	}
	if err = e.verifyRemote(rel, *check, false); err != nil {
		return err
	}
	present := map[string]bool{}
	for _, a := range rel.Assets {
		present[a.Name] = true
	}
	names := make([]string, 0, len(r.Files))
	for name := range r.Files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if !present[name] {
			if _, err = e.client.run([]string{"release", "upload", e.tag, filepath.Join(stage, name), "--repo", e.client.repo}); err != nil {
				return err
			}
		}
	}
	rel, check, err = e.inspect()
	if err != nil {
		return err
	}
	if rel == nil || !reflect.DeepEqual(*check, r) {
		return fmt.Errorf("release provenance changed after upload")
	}
	if err = e.verifyRemote(rel, *check, true); err != nil {
		return err
	}
	if rel.Draft {
		if _, err = e.client.run([]string{"release", "edit", e.tag, "--repo", e.client.repo, "--draft=false"}); err != nil {
			return err
		}
	}
	final, record, err := e.inspect()
	if err != nil {
		return err
	}
	if final == nil || final.Draft || !reflect.DeepEqual(*record, r) {
		return fmt.Errorf("release was not published with the expected provenance")
	}
	return e.verifyRemote(final, *record, true)
}
func Run(root string, args []string) error { return run(root, args, command) }
func run(root string, args []string, execute runner) error {
	if len(args) == 0 {
		return fmt.Errorf("expected preflight, collect or publish")
	}
	fs := flag.NewFlagSet(args[0], flag.ContinueOnError)
	tag := fs.String("tag", "", "product tag, e.g. v0.2.0")
	repo := fs.String("repo", os.Getenv("GITHUB_REPOSITORY"), "owner/repository")
	output := fs.String("output", "", "GitHub output file")
	input := fs.String("artifacts", "", "cli/plugin/vscode input directories")
	out := fs.String("out", "", "empty collection output directory")
	version := fs.String("version", "", "collection version")
	sha := fs.String("sha", "", "collection commit")
	runID := fs.Int64("run-id", 0, "source workflow run ID")
	attempt := fs.Int("attempt", 0, "publisher attempt")
	cliID := fs.String("cli-artifact", "", "exact CLI artifact ID")
	pluginID := fs.String("plugin-artifact", "", "exact plugin artifact ID")
	vscodeID := fs.String("vscode-artifact", "", "exact VSIX artifact ID")
	dryRun := fs.Bool("dry-run", false, "verify without release writes")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments")
	}
	if args[0] == "collect" {
		if err := versioning.Validate(*version); err != nil {
			return err
		}
		actual, err := versioning.Check(root)
		if err != nil {
			return err
		}
		if actual != *version || !shaRE.MatchString(*sha) || *input == "" || *out == "" {
			return fmt.Errorf("collect requires matching version, commit, input and output")
		}
		hashes, err := collect(root, *version, *sha, *input, *out)
		if err != nil {
			return err
		}
		fmt.Printf("PASS: staged %d verified release files\n", len(hashes))
		return nil
	}
	if args[0] != "preflight" && args[0] != "publish" {
		return fmt.Errorf("unknown release subcommand: %s", args[0])
	}
	v, commit, err := resolve(root, *tag)
	if err != nil {
		return err
	}
	if *repo == "" {
		raw, err := execute([]string{"repo", "view", "--json", "nameWithOwner", "--jq", ".nameWithOwner"})
		if err != nil {
			return err
		}
		*repo = strings.TrimSpace(string(raw))
	}
	if !repoRE.MatchString(*repo) || strings.Contains(*repo, "/..") {
		return fmt.Errorf("invalid repository")
	}
	e := &engine{root: root, version: v, sha: commit, tag: *tag, client: newClient(*repo, execute)}
	if args[0] == "preflight" {
		mode, err := e.preflight()
		if err != nil {
			return err
		}
		data := fmt.Sprintf("mode=%s\nversion=%s\nsha=%s\n", mode, v, commit)
		if *output != "" {
			f, err := os.OpenFile(*output, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
			if err != nil {
				return err
			}
			_, err = f.WriteString(data)
			closeErr := f.Close()
			if err != nil {
				return err
			}
			if closeErr != nil {
				return closeErr
			}
		} else {
			fmt.Print(data)
		}
		return nil
	}
	ids := map[string]int64{}
	// Resume/published paths intentionally accept missing current producer outputs.
	rel, _, err := e.inspect()
	if err != nil {
		return err
	}
	if rel == nil {
		for component, value := range map[string]string{"cli": *cliID, "plugin": *pluginID, "vscode": *vscodeID} {
			id, err := strconv.ParseInt(value, 10, 64)
			if err != nil || id < 1 {
				return fmt.Errorf("positive %s artifact ID required", component)
			}
			ids[component] = id
		}
	}
	return e.publish(*input, *runID, *attempt, ids, *dryRun)
}
