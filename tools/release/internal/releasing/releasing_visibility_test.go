// Copyright 2026 lemonadern
// SPDX-License-Identifier: Apache-2.0
package releasing

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

type visibilityHarness struct {
	created bool
	reads   int
	hide    int
	sleeps  []time.Duration
	onList  func(int) error
}

func delayedVisibility(f *fakeGH, e *engine, hiddenReads int) *visibilityHarness {
	h := &visibilityHarness{hide: hiddenReads}
	base := e.client.run
	e.sleep = func(delay time.Duration) { h.sleeps = append(h.sleeps, delay) }
	e.client.run = func(args []string) ([]byte, error) {
		if h.created {
			for _, arg := range args {
				if strings.Contains(arg, "/releases?") {
					h.reads++
					if h.onList != nil {
						if err := h.onList(h.reads); err != nil {
							return nil, err
						}
					}
					if h.reads <= h.hide {
						return []byte("[]"), nil
					}
					break
				}
			}
		}
		out, err := base(args)
		if err == nil && len(args) > 1 && args[0] == "release" && args[1] == "create" {
			h.created = true
		}
		return out, err
	}
	return h
}
func onlyCreated(t *testing.T, f *fakeGH) {
	t.Helper()
	if !reflect.DeepEqual(f.writes, []string{"create"}) {
		t.Fatalf("unexpected release mutations: %v", f.writes)
	}
}
func TestNewDraftWaitsForListVisibilityWithoutCreatingTwice(t *testing.T) {
	for _, component := range []string{"plugin", "cli", "vscode"} {
		t.Run(component, func(t *testing.T) {
			f, e := componentFake(t, component)
			h := delayedVisibility(f, e, 2)
			if err := e.publish("", testRun, 1, f.ids(), false); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(h.sleeps, []time.Duration{time.Second, 2 * time.Second}) {
				t.Fatal("wrong backoff", h.sleeps)
			}
			creates := 0
			for _, write := range f.writes {
				if write == "create" {
					creates++
				}
			}
			if creates != 1 || f.rel.Draft || len(f.rel.Assets) != len(componentFiles(component, testVersion)) {
				t.Fatal("draft not published exactly once", f.writes)
			}
		})
	}
}
func TestNewDraftVisibilityWaitHasSixtySecondBudgetAndCanResume(t *testing.T) {
	f, e := componentFake(t, "cli")
	h := delayedVisibility(f, e, 100)
	err := e.publish("", testRun, 1, f.ids(), false)
	if err == nil || !strings.Contains(err.Error(), "60 seconds of waiting") {
		t.Fatal("missing bounded visibility failure", err)
	}
	want := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 15 * time.Second, 30 * time.Second}
	if !reflect.DeepEqual(h.sleeps, want) || h.reads != 7 {
		t.Fatal("wrong retry budget", h.sleeps, h.reads)
	}
	var total time.Duration
	for _, delay := range h.sleeps {
		total += delay
	}
	if total != time.Minute {
		t.Fatal(total)
	}
	onlyCreated(t, f)
	if f.rel == nil || !f.rel.Draft || len(f.rel.Assets) != 0 {
		t.Fatal("visibility failure changed the original empty draft")
	}
	// Once the original draft is visible, a separate rerun uses its recorded
	// source artifacts and does not retry create or the publication itself.
	h.created = false
	f.writes = nil
	if err := e.publish("", testRun, 2, nil, false); err != nil {
		t.Fatal(err)
	}
	for _, write := range f.writes {
		if write == "create" {
			t.Fatal("created again during resume")
		}
	}
	if f.rel.Draft {
		t.Fatal("did not resume original draft")
	}
}
func TestNewDraftVisibilityDoesNotRetryAPIOrVisibleProvenanceErrors(t *testing.T) {
	for _, kind := range []string{"api-error", "malformed-record", "stale-record", "changed-record", "tag-error", "published-corruption"} {
		t.Run(kind, func(t *testing.T) {
			f, e := componentFake(t, "vscode")
			h := delayedVisibility(f, e, 0)
			h.onList = func(read int) error {
				switch kind {
				case "api-error":
					return fmt.Errorf("simulated GitHub API error")
				case "malformed-record":
					f.rel.Body = "not automation provenance"
				case "stale-record", "changed-record":
					r, err := parseRecord(f.rel.Body, testVersion, testSHA)
					if err != nil {
						return err
					}
					if kind == "stale-record" {
						r.Attempt++
					} else {
						for name := range r.Files {
							r.Files[name] = strings.Repeat("b", 64)
							break
						}
					}
					data, err := json.Marshal(r)
					if err != nil {
						return err
					}
					f.rel.Body = markerStart + string(data) + markerEnd
				case "published-corruption":
					f.rel.Draft = false
					for name, data := range f.data {
						f.attach(name, data)
					}
					for id := range f.remote {
						f.remote[id] = []byte("corrupt")
						break
					}
				}
				return nil
			}
			if kind == "tag-error" {
				base := e.client.run
				e.client.run = func(args []string) ([]byte, error) {
					if h.created {
						for _, arg := range args {
							if strings.Contains(arg, "/git/tags/") {
								return nil, fmt.Errorf("remote tag changed")
							}
						}
					}
					return base(args)
				}
			}
			if err := e.publish("", testRun, 1, f.ids(), false); err == nil {
				t.Fatal("accepted", kind)
			}
			if len(h.sleeps) != 0 {
				t.Fatal("retried a non-visibility failure", h.sleeps)
			}
			onlyCreated(t, f)
		})
	}
}
func TestExistingDraftDisappearanceIsNotRetriedOrRecreated(t *testing.T) {
	f, e := componentFake(t, "vscode")
	if err := e.publish("", testRun, 1, f.ids(), false); err != nil {
		t.Fatal(err)
	}
	f.rel.Draft = true
	f.writes = nil
	h := delayedVisibility(f, e, 0)
	f.collectHook = func() { f.rel = nil }
	if err := e.publish("", testRun, 2, nil, false); err == nil || !strings.Contains(err.Error(), "release disappeared before upload") {
		t.Fatal("accepted disappeared existing release", err)
	}
	if len(h.sleeps) != 0 {
		t.Fatal("retried an existing release deletion")
	}
	noWrites(t, f)
}
func TestCreatedDraftDisappearanceAfterUploadIsNotRetried(t *testing.T) {
	f, e := componentFake(t, "vscode")
	h := delayedVisibility(f, e, 0)
	// The first list finds the just-created draft. If it later disappears, that
	// must not be treated as the initial create's eventual visibility delay.
	h.onList = func(read int) error {
		if read == 2 {
			f.rel = nil
		}
		return nil
	}
	if err := e.publish("", testRun, 1, f.ids(), false); err == nil {
		t.Fatal("accepted disappearance after upload")
	}
	if len(h.sleeps) != 0 {
		t.Fatal("retried later disappearance")
	}
	creates := 0
	for _, write := range f.writes {
		if write == "create" {
			creates++
		}
		if write == "edit" {
			t.Fatal("published disappeared release")
		}
	}
	if creates != 1 {
		t.Fatal(f.writes)
	}
}

func TestNewDraftRejectsChangedProvenanceWhenFinallyVisible(t *testing.T) {
	f, e := componentFake(t, "plugin")
	h := delayedVisibility(f, e, 2)
	h.onList = func(read int) error {
		if read <= h.hide {
			return nil
		}
		r, err := parseRecord(f.rel.Body, testVersion, testSHA)
		if err != nil {
			return err
		}
		r.Attempt++
		data, err := json.Marshal(r)
		if err != nil {
			return err
		}
		f.rel.Body = markerStart + string(data) + markerEnd
		return nil
	}
	if err := e.publish("", testRun, 1, f.ids(), false); err == nil || !strings.Contains(err.Error(), "provenance changed before upload") {
		t.Fatal("accepted changed record after visibility wait", err)
	}
	if !reflect.DeepEqual(h.sleeps, []time.Duration{time.Second, 2 * time.Second}) {
		t.Fatal("retried a visible stale record", h.sleeps)
	}
	onlyCreated(t, f)
}
func TestFailedDraftCreationIsNotRetried(t *testing.T) {
	f, e := componentFake(t, "cli")
	h := delayedVisibility(f, e, 2)
	base := e.client.run
	creates := 0
	e.client.run = func(args []string) ([]byte, error) {
		if len(args) > 1 && args[0] == "release" && args[1] == "create" {
			creates++
			return nil, fmt.Errorf("create request failed")
		}
		return base(args)
	}
	if err := e.publish("", testRun, 1, f.ids(), false); err == nil {
		t.Fatal("accepted failed create")
	}
	if creates != 1 || len(h.sleeps) != 0 {
		t.Fatal("retried create failure", creates, h.sleeps)
	}
	noWrites(t, f)
}
