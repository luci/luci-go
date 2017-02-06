// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package tracer

import (
	"bytes"
	"encoding/json"
	"os"
	"sort"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func ExampleSpan() {
	// Open a file with os.Create().
	if err := Start(&bytes.Buffer{}, 0); err != nil {
		defer Stop()
	}

	// Do stuff.
	var err error

	end := Span(nil, "action1", Args{"foo": "bar"})
	defer func() { end(Args{"err": err}) }()
}

func ExampleInstant() {
	// Open a file with os.Create().
	if err := Start(&bytes.Buffer{}, 0); err != nil {
		defer Stop()
	}
	Instant(nil, "explosion", Global, Args{"level": "hard"})
}

func ExampleNewPID() {
	// Open a file with os.Create().
	if err := Start(&bytes.Buffer{}, 0); err != nil {
		defer Stop()
	}

	// Logging to sub will use a different group in the UI.
	key := new(int)
	NewPID(key, "main")
	Instant(key, "explosion", Process, Args{"level": "hard"})
}

func TestNotStarted(t *testing.T) {
	// Must not crash.
	Instant(nil, "", Global, nil)
	Span(nil, "", nil)(nil)
	NewPID(nil, "")
}

func TestCounterAdd(t *testing.T) {
	Convey(`A tracer counter should support being incremented.`, t, func() {
		b := &bytes.Buffer{}
		So(Start(b, 1), ShouldBeNil)
		CounterAdd(nil, "explosion", 42)
		CounterAdd(nil, "explosion", 3)
		Stop()

		check(t, b, []event{
			{
				Type: eventCounter,
				Name: "explosion",
				Args: Args{"value": 42.},
				ID:   1,
			},
			{
				Type: eventCounter,
				Name: "explosion",
				Args: Args{"value": 45.},
				ID:   2,
			},
		})
	})
}

func TestCounterSet(t *testing.T) {
	Convey(`A tracer counter should support being set.`, t, func() {
		b := &bytes.Buffer{}
		So(Start(b, 1), ShouldBeNil)
		CounterSet(nil, "explosion", 42)
		CounterSet(nil, "explosion", 3)
		Stop()

		check(t, b, []event{
			{
				Type: eventCounter,
				Name: "explosion",
				Args: Args{"value": 42.},
				ID:   1,
			},
			{
				Type: eventCounter,
				Name: "explosion",
				Args: Args{"value": 3.},
				ID:   2,
			},
		})
	})
}

func TestInstant(t *testing.T) {
	Convey(`A tracer should support instantaneous events with no duration.`, t, func() {
		b := &bytes.Buffer{}
		So(Start(b, 1), ShouldBeNil)
		Instant(nil, "explosion", Global, Args{"level": "hard"})
		Stop()

		check(t, b, []event{
			{
				Type:     eventNestableInstant,
				Category: "ignored",
				Name:     "explosion",
				Args:     Args{"level": "hard"},
				Scope:    Global,
				ID:       1,
			},
		})
	})
}

func TestSpanSimpleBegin(t *testing.T) {
	Convey(`A tracer should support spanning event with duration.`, t, func() {
		b := &bytes.Buffer{}
		So(Start(b, 1), ShouldBeNil)
		Span(nil, "action1", Args{"err": "bar"})(nil)
		Stop()

		check(t, b, []event{
			{
				Type:     eventNestableBegin,
				Category: "ignored",
				Name:     "action1",
				Args:     Args{"err": "bar"},
				ID:       1,
			},
			{
				Type:     eventNestableEnd,
				Category: "ignored",
				Name:     "action1",
				Args:     consts.fakeArgs,
				ID:       1,
			},
		})
	})
}

func TestSpanSimpleEnd(t *testing.T) {
	Convey(`A tracer should support spanning event with duration.`, t, func() {
		b := &bytes.Buffer{}
		So(Start(b, 1), ShouldBeNil)
		Span(nil, "action1", nil)(Args{"err": "bar"})
		Stop()

		check(t, b, []event{
			{
				Type:     eventNestableBegin,
				Category: "ignored",
				Name:     "action1",
				Args:     consts.fakeArgs,
				ID:       1,
			},
			{
				Type:     eventNestableEnd,
				Category: "ignored",
				Name:     "action1",
				Args:     Args{"err": "bar"},
				ID:       1,
			},
		})
	})
}

// Private details.

type traceContext struct {
	Args []string `json:"args"`
	Wd   string   `json:"cwd"`
}

type events []event

func (e events) Len() int { return len(e) }
func (e events) Less(i, j int) bool {
	if e[i].Timestamp != e[j].Timestamp {
		return e[i].Timestamp < e[j].Timestamp
	}
	// For Counter operations.
	return e[i].ID < e[j].ID
}
func (e events) Swap(i, j int) { e[i], e[j] = e[j], e[i] }

type traceFile struct {
	Context traceContext `json:"context"`
	Events  events       `json:"traceEvents"`
}

func check(t *testing.T, b *bytes.Buffer, expected []event) {
	Convey(`Basic checks performed for many tracer tests.`, func() {
		actual := &traceFile{}
		So(json.Unmarshal(b.Bytes(), actual), ShouldBeNil)
		// First sort by .Timestamp. Then Zap out .Timestamp since it is not
		// deterministic. Convert Duration to binary value, either 0 or 1 since it's
		// value is either set or not set.
		sort.Sort(actual.Events)
		for i := range actual.Events {
			// Timestamp can be zero on low resolution clock (e.g. Windows) when an
			// event is filed right after tracer.Start(). Using high resolution (1ms)
			// clock resolution on Windows is optional.
			So(actual.Events[i].Timestamp, ShouldBeGreaterThanOrEqualTo, 0)
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
		So(actual.Context, ShouldResemble, e.Context)
		So(actual.Events, ShouldResemble, e.Events)
		So(actual, ShouldResemble, e)
	})
}
