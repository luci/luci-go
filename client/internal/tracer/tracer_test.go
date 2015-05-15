// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package tracer

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"github.com/maruel/ut"
)

func ExampleSpan() {
	// Open a file with os.Create().
	if err := Start(&bytes.Buffer{}, 0); err != nil {
		defer Stop()
	}

	// Do stuff.
	var err error

	end := Span(nil, "component1", "action1", Args{"foo": "bar"})
	defer func() { end(Args{"err": err}) }()
}

func ExampleInstant() {
	// Open a file with os.Create().
	if err := Start(&bytes.Buffer{}, 0); err != nil {
		defer Stop()
	}
	Instant(nil, "component1", "explosion", Global, Args{"level": "hard"})
}

func ExampleNewPID() {
	// Open a file with os.Create().
	if err := Start(&bytes.Buffer{}, 0); err != nil {
		defer Stop()
	}

	// Logging to sub will use a different group in the UI.
	key := new(int)
	NewPID(key, "subproc", "main")
	Instant(key, "component1", "explosion", Process, Args{"level": "hard"})
}

func ExampleNewTID() {
	// Open a file with os.Create().
	if err := Start(&bytes.Buffer{}, 0); err != nil {
		defer Stop()
	}

	// Logging to sub will use a different line within the same group of the
	// parent in the UI.
	key := new(int)
	NewTID(key, nil, "I/O")
	Instant(key, "component1", "explosion", Process, Args{"level": "hard"})
}

func TestNotStarted(t *testing.T) {
	// Must not crash.
	Instant(nil, "", "", Global, nil)
	Span(nil, "", "", nil)(nil)
	NewPID(nil, "", "")
	NewTID(nil, nil, "")
}

func TestInstant(t *testing.T) {
	b := &bytes.Buffer{}
	ut.AssertEqual(t, nil, Start(b, 1))
	Instant(nil, "component1", "explosion", Global, Args{"level": "hard"})
	Stop()

	check(t, b, []event{
		{
			Type:     eventInstant,
			Category: "component1",
			Name:     "explosion",
			Args:     Args{"level": "hard"},
			Scope:    "g",
		},
	})
}

func TestSpanSimpleBegin(t *testing.T) {
	b := &bytes.Buffer{}
	ut.AssertEqual(t, nil, Start(b, 1))
	Span(nil, "component1", "action1", Args{"err": "bar"})(nil)
	Stop()

	check(t, b, []event{
		{
			Type:     eventComplete,
			Category: "component1",
			Name:     "action1",
			Args:     Args{"err": "bar"},
			Duration: 1,
		},
	})
}

func TestSpanSimpleEnd(t *testing.T) {
	b := &bytes.Buffer{}
	ut.AssertEqual(t, nil, Start(b, 1))
	Span(nil, "component1", "action1", nil)(Args{"err": "bar"})
	Stop()

	check(t, b, []event{
		{
			Type:     eventComplete,
			Category: "component1",
			Name:     "action1",
			Args:     Args{"err": "bar"},
			Duration: 1,
		},
	})
}

func TestNewPIDNewTID(t *testing.T) {
	b := &bytes.Buffer{}
	ut.AssertEqual(t, nil, Start(b, 1))

	pidKey := new(int)
	tidKey := new(int)
	NewPID(pidKey, "subproc", "main")
	NewTID(tidKey, pidKey, "I/O")

	Instant(pidKey, "component1", "", Global, nil)
	Instant(tidKey, "component2", "", Global, nil)

	Discard(tidKey)
	Instant(tidKey, "component3", "", Global, nil)

	Stop()

	check(t, b, []event{
		{
			Pid:  2,
			Type: eventMetadata,
			Name: string(processName),
			Args: Args{"name": "subproc"},
		},
		{
			Pid:  2,
			Type: eventMetadata,
			Name: string(threadName),
			Args: Args{"name": "main"},
		},
		{
			Pid:  2,
			Tid:  2,
			Type: eventMetadata,
			Name: string(threadName),
			Args: Args{"name": "I/O"},
		},
		{
			Pid:      2,
			Type:     eventInstant,
			Category: "component1",
			Scope:    "g",
		},
		{
			Pid:      2,
			Tid:      2,
			Type:     eventInstant,
			Category: "component2",
			Scope:    "g",
		},
		{
			Type:     eventInstant,
			Category: "component3",
			Scope:    "g",
		},
	})
}

// Private details.

type traceContext struct {
	Args []string `json:"args"`
	Wd   string   `json:"cwd"`
}

type traceFile struct {
	Context traceContext `json:"context"`
	Events  []event      `json:"traceEvents"`
}

func check(t *testing.T, b *bytes.Buffer, expected []event) {
	actual := &traceFile{}
	ut.AssertEqual(t, nil, json.Unmarshal(b.Bytes(), actual))
	// Zap out .Timestamp since it is not deterministic. Convert Duration to
	// binary value, either 0 or 1 since it's value is either set or not set.
	for i := range actual.Events {
		ut.AssertEqual(t, true, actual.Events[i].Timestamp >= 0)
		actual.Events[i].Timestamp = 0
		if actual.Events[i].Duration != 0 {
			actual.Events[i].Duration = 1
		}
	}
	for i := range expected {
		if expected[i].Pid == 0 {
			expected[i].Pid = 1
		}
		if expected[i].Tid == 0 {
			expected[i].Tid = 1
		}
	}
	wd, _ := os.Getwd()
	e := &traceFile{traceContext{os.Args, wd}, expected}
	ut.AssertEqual(t, e.Context, actual.Context)
	ut.AssertEqual(t, e.Events, actual.Events)
	ut.AssertEqual(t, e, actual)
}
