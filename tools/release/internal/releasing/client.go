// Copyright 2026 lemonadern
// SPDX-License-Identifier: Apache-2.0
package releasing

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type runner func([]string) ([]byte, error)
type client struct {
	repo       string
	run        runner
	binaryFlag bool
}

func command(args []string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "gh", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err = cmd.Start(); err != nil {
		return nil, fmt.Errorf("start gh: %w", err)
	}
	data, readErr := io.ReadAll(io.LimitReader(stdout, (128<<20)+1))
	if len(data) > 128<<20 || readErr != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, fmt.Errorf("gh response exceeded limit or could not be read")
	}
	if err = cmd.Wait(); err != nil {
		// Do not relay HTTP responses, signed download URLs or CLI diagnostics
		// into publication logs. They can contain transient credentials.
		return nil, fmt.Errorf("gh %s failed (%v); check repository access and resource availability", args[0], err)
	}
	return data, nil
}
func newClient(repo string, run runner) *client {
	c := &client{repo: repo, run: run}
	help, err := run([]string{"api", "--help"})
	c.binaryFlag = err == nil && bytes.Contains(help, []byte("--allow-escape-sequences"))
	return c
}
func (c *client) api(endpoint string, result any) error {
	raw, err := c.run([]string{"api", "--method", "GET", "repos/" + c.repo + "/" + endpoint})
	if err != nil {
		return err
	}
	if err = json.Unmarshal(raw, result); err != nil {
		return fmt.Errorf("decode GitHub response: %w", err)
	}
	return nil
}
func (c *client) binary(endpoint, accept string) ([]byte, error) {
	args := []string{"api", "--method", "GET", "repos/" + c.repo + "/" + endpoint, "--header", "Accept: " + accept}
	if c.binaryFlag {
		args = append(args, "--allow-escape-sequences")
	}
	return c.run(args)
}

type asset struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Size int64  `json:"size"`
}
type release struct {
	ID         int64   `json:"id"`
	Tag        string  `json:"tag_name"`
	Draft      bool    `json:"draft"`
	Prerelease bool    `json:"prerelease"`
	Body       string  `json:"body"`
	Assets     []asset `json:"assets"`
}

func (c *client) findRelease(tag string) (*release, error) {
	var found *release
	for page := 1; page <= 100; page++ {
		var list []release
		if err := c.api("releases?per_page=100&page="+strconv.Itoa(page), &list); err != nil {
			return nil, err
		}
		for _, r := range list {
			if r.Tag == tag {
				if found != nil {
					return nil, fmt.Errorf("multiple releases use %s", tag)
				}
				copy := r
				found = &copy
			}
		}
		if len(list) < 100 {
			return found, nil
		}
	}
	return nil, fmt.Errorf("release listing exceeded pagination limit")
}
func (c *client) assetData(a asset) ([]byte, error) {
	if a.ID <= 0 || a.Size <= 0 || a.Size > 128<<20 {
		return nil, fmt.Errorf("invalid release asset metadata: %s", a.Name)
	}
	data, err := c.binary("releases/assets/"+strconv.FormatInt(a.ID, 10), "application/octet-stream")
	if err != nil {
		return nil, err
	}
	if int64(len(data)) != a.Size {
		return nil, fmt.Errorf("release asset size mismatch: %s", a.Name)
	}
	return data, nil
}

type artifactRef struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	RunID   int64  `json:"run_id"`
	Attempt int    `json:"attempt"`
	Digest  string `json:"digest"`
}
type artifactMeta struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Expired  bool   `json:"expired"`
	Digest   string `json:"digest"`
	Workflow struct {
		ID  int64  `json:"id"`
		SHA string `json:"head_sha"`
	} `json:"workflow_run"`
}

var artifactPrefixes = map[string]string{"cli": "cli-distributions", "plugin": "plugin-update-site", "vscode": "vscode-vsix"}

func (c *client) artifact(id, runID int64, sha, component string) (artifactRef, error) {
	var a artifactMeta
	if id <= 0 || runID <= 0 {
		return artifactRef{}, fmt.Errorf("positive artifact and run IDs required")
	}
	if err := c.api("actions/artifacts/"+strconv.FormatInt(id, 10), &a); err != nil {
		return artifactRef{}, err
	}
	prefix := artifactPrefixes[component] + "-" + strconv.FormatInt(runID, 10) + "-"
	if a.ID != id || a.Expired || a.Workflow.ID != runID || a.Workflow.SHA != sha || !strings.HasPrefix(a.Name, prefix) {
		return artifactRef{}, fmt.Errorf("artifact metadata does not match %s run and commit", component)
	}
	attempt, err := strconv.Atoi(strings.TrimPrefix(a.Name, prefix))
	if err != nil || attempt < 1 || a.Name != prefix+strconv.Itoa(attempt) {
		return artifactRef{}, fmt.Errorf("invalid producer attempt: %s", a.Name)
	}
	if !strings.HasPrefix(a.Digest, "sha256:") || !hashRE.MatchString(strings.TrimPrefix(a.Digest, "sha256:")) {
		return artifactRef{}, fmt.Errorf("artifact has no valid SHA-256 digest")
	}
	return artifactRef{ID: a.ID, Name: a.Name, RunID: runID, Attempt: attempt, Digest: a.Digest}, nil
}
func (c *client) verifyRun(id int64, sha, tag string, attempt int, artifacts map[string]artifactRef) error {
	var run struct {
		SHA     string `json:"head_sha"`
		Event   string `json:"event"`
		Branch  string `json:"head_branch"`
		Path    string `json:"path"`
		Attempt int    `json:"run_attempt"`
	}
	if err := c.api("actions/runs/"+strconv.FormatInt(id, 10), &run); err != nil {
		return err
	}
	if run.SHA != sha || run.Event != "push" || run.Branch != tag || run.Path != ".github/workflows/release.yml" || attempt < 1 || run.Attempt < attempt {
		return fmt.Errorf("release run is not a push of this tag and commit")
	}
	type job struct {
		ID         int64  `json:"id"`
		Name       string `json:"name"`
		Status     string `json:"status"`
		Conclusion string `json:"conclusion"`
		Attempt    int    `json:"run_attempt"`
	}
	latest := map[string]job{}
	for page := 1; page <= 100; page++ {
		var result struct {
			Jobs []job `json:"jobs"`
		}
		if err := c.api("actions/runs/"+strconv.FormatInt(id, 10)+"/jobs?filter=all&per_page=100&page="+strconv.Itoa(page), &result); err != nil {
			return err
		}
		for _, j := range result.Jobs {
			if j.Attempt >= 1 && j.Attempt <= attempt && (j.Attempt > latest[j.Name].Attempt || (j.Attempt == latest[j.Name].Attempt && j.ID > latest[j.Name].ID)) {
				latest[j.Name] = j
			}
		}
		if len(result.Jobs) < 100 {
			break
		}
		if page == 100 {
			return fmt.Errorf("job listing exceeded pagination limit")
		}
	}
	required := []string{"CLI / ubuntu-24.04", "CLI / macos-15", "CLI / windows-2025", "CLI / Package", "VSIX / Package", "VSIX / ubuntu-24.04", "VSIX / macos-15", "VSIX / windows-2025", "Real DBeaver CE 26.2.2 / Windows x64", "Go install / Tagged module", "Release assets / Validate assembly"}
	component := ""
	if strings.Contains(tag, "/v") {
		component, _ = tagComponent(tag)
	}
	if component != "" && component != "cli" {
		filtered := required[:0]
		for _, name := range required {
			if name != "Go install / Tagged module" {
				filtered = append(filtered, name)
			}
		}
		required = filtered
	}
	selected := map[string]job{}
	for _, suffix := range required {
		matches := 0
		for name, j := range latest {
			if name == suffix || strings.HasSuffix(name, " / "+suffix) {
				matches++
				if j.Status != "completed" || j.Conclusion != "success" {
					return fmt.Errorf("required release job did not succeed: %s", name)
				}
				selected[suffix] = j
			}
		}
		if matches != 1 {
			return fmt.Errorf("expected one successful job for %s, got %d", suffix, matches)
		}
	}
	producerJobs := map[string]string{"cli": "CLI / Package", "plugin": "Real DBeaver CE 26.2.2 / Windows x64", "vscode": "VSIX / Package"}
	maxProducer := 0
	for component, name := range producerJobs {
		if _, ok := artifacts[component]; !ok && len(artifacts) == 1 {
			continue
		}
		source, ok := artifacts[component]
		if !ok || source.Attempt < 1 || source.Attempt > attempt || selected[name].Attempt != source.Attempt {
			return fmt.Errorf("artifact producer does not match validated job attempt: %s", component)
		}
		if source.Attempt > maxProducer {
			maxProducer = source.Attempt
		}
	}
	for _, consumer := range []string{"VSIX / ubuntu-24.04", "VSIX / macos-15", "VSIX / windows-2025", "Real DBeaver CE 26.2.2 / Windows x64"} {
		if selected[consumer].Attempt < artifacts["vscode"].Attempt {
			return fmt.Errorf("VSIX test predates the recorded VSIX producer: %s", consumer)
		}
	}
	if selected["Release assets / Validate assembly"].Attempt < maxProducer {
		return fmt.Errorf("assembly validation predates a recorded producer")
	}
	return nil
}
