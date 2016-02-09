// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package annotation

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/golang/protobuf/proto"
	"github.com/luci/luci-go/common/clock/testclock"
	"github.com/luci/luci-go/common/logdog/types"
	"github.com/luci/luci-go/common/proto/milo"
	. "github.com/smartystreets/goconvey/convey"
)

func normalize(s string) string {
	return strings.Map(func(r rune) rune {
		if r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)) {
			return r
		}
		return '_'
	}, s)
}

type annotationReplayEngine struct {
	*testing.T
	tc  testclock.TestClock
	err string
}

// playAnnotationScript loads an annotation script from "path" and plays it
// through the supplied State line-by-line.
//
// Empty lines and lines beginning with "#" are ignored. Preceding whitespace
// is discarded.
func (e *annotationReplayEngine) playAnnotationScript(s *State, name string) error {
	path := filepath.Join("testdata", fmt.Sprintf("%s.annotations.txt", normalize(name)))
	f, err := os.Open(path)
	if err != nil {
		e.Errorf("Failed to open annotations source [%s]: %v", path, err)
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		// Trim, discard empty lines and comment lines.
		line := strings.TrimLeftFunc(scanner.Text(), unicode.IsSpace)
		if len(line) == 0 || strings.HasPrefix(line, "#") {
			continue
		}

		switch {
		case line == "+time":
			e.tc.Add(1 * time.Second)

		case strings.HasPrefix(line, "+error"):
			e.err = strings.SplitN(line, " ", 2)[1]

		default:
			// Annotation.
			err := s.Append(line)
			if exp := e.err; exp != "" {
				e.err = ""
				if err == nil {
					return fmt.Errorf("expected error, but didn't encounter it: %q", e.err)
				}
				if !strings.Contains(err.Error(), e.err) {
					return fmt.Errorf("expected error %q, but got: %v", e.err, err)
				}
			} else if err != nil {
				return err
			}
		}
	}

	return nil
}

func loadStepProto(t *testing.T, test, name string) *milo.Step {
	path := filepath.Join("testdata", fmt.Sprintf("%s_%s.proto.txt", normalize(test), normalize(name)))
	data, err := ioutil.ReadFile(path)
	if err != nil {
		t.Errorf("Failed to read milo.Step proto [%s]: %v", path, err)
		return nil
	}

	st := milo.Step{}
	if err := proto.UnmarshalText(string(data), &st); err != nil {
		t.Errorf("Failed to Unmarshal milo.Step proto [%s]: %v", path, err)
		return nil
	}
	return &st
}

func loadLogText(t *testing.T, test, name string) []string {
	path := filepath.Join("testdata", fmt.Sprintf("%s_%s.txt", normalize(test), normalize(name)))
	f, err := os.Open(path)
	if err != nil {
		t.Errorf("Failed to open log lines [%s]: %v", path, err)
		return nil
	}
	defer f.Close()

	lines := []string(nil)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines
}

// testCallbacks implements the Callbacks interface, retaining all callback
// data in memory.
type testCallbacks struct {
	// closed is the set of steps that have been closed, keyed on step
	// CanonicalName.
	closed map[string]struct{}

	// protos is the cumulative set of marshalled emitted protobuf state for each
	// step, keyed on step CanonicalName.
	protos map[string][][]byte

	// logs is the content of emitted annotation logs, keyed on stream name.
	logs map[types.StreamName][]string
	// logsOpen tracks whether a given annotation log is open.
	logsOpen map[types.StreamName]struct{}
}

func (tc *testCallbacks) StepClosed(s *Step) {
	tc.closed[s.CanonicalName()] = struct{}{}
}

func (tc *testCallbacks) Updated(s *Step) {
	data, err := proto.Marshal(s.Proto())
	if err != nil {
		panic(err)
	}
	tc.protos[s.CanonicalName()] = append(tc.protos[s.CanonicalName()], data)
}

func (tc *testCallbacks) StepLogLine(s *Step, n types.StreamName, line string) {
	if _, ok := tc.logs[n]; ok {
		// The log exists. Assert that it is open.
		if _, ok := tc.logsOpen[n]; !ok {
			panic(fmt.Errorf("write to closed log stream: %q", n))
		}
	}

	tc.logsOpen[n] = struct{}{}
	tc.logs[n] = append(tc.logs[n], line)
}

func (tc *testCallbacks) StepLogEnd(s *Step, n types.StreamName) {
	if _, ok := tc.logsOpen[n]; !ok {
		panic(fmt.Errorf("close of closed log stream: %q", n))
	}
	delete(tc.logsOpen, n)
}

func (tc *testCallbacks) lastProto(name string) *milo.Step {
	protoList := tc.protos[name]
	if len(protoList) == 0 {
		return nil
	}

	s := milo.Step{}
	if err := proto.Unmarshal(protoList[len(protoList)-1], &s); err != nil {
		panic(fmt.Errorf("failed to unmarshal snapshot: %v", err))
	}
	return &s
}

func TestState(t *testing.T) {
	t.Parallel()

	Convey(`A testing annotation State`, t, func() {
		tc := testclock.New(time.Date(2015, 1, 1, 0, 0, 0, 0, time.UTC))
		cb := testCallbacks{
			closed:   map[string]struct{}{},
			protos:   map[string][][]byte{},
			logs:     map[types.StreamName][]string{},
			logsOpen: map[types.StreamName]struct{}{},
		}
		exe := Execution{
			Name:    "testcommand",
			Command: []string{"testcommand", "foo", "bar"},
			Dir:     "/path/to/dir",
			Env: map[string]string{
				"FOO": "BAR",
				"BAZ": "QUX",
			},
		}

		s := State{
			LogNameBase: types.StreamName("base"),
			Callbacks:   &cb,
			Clock:       tc,
		}

		for _, testCase := range []struct {
			name string
			exe  *Execution
		}{
			{"default", &exe},
			{"coverage", nil},
		} {
			Convey(fmt.Sprintf(`Correctly loads/generates for %q test case.`, testCase.name), func() {
				r := annotationReplayEngine{
					T:  t,
					tc: tc,
				}
				s.Execution = testCase.exe
				So(r.playAnnotationScript(&s, testCase.name), ShouldBeNil)

				// Iterate through generated streams and validate.
				s.Finish()

				// All log streams should be closed.
				So(cb.logsOpen, ShouldResemble, map[types.StreamName]struct{}{})

				// Iterate over each generated stream and assert that it matches its
				// expectation. Do it deterministically so failures aren't frustrating
				// to reproduce.
				s.ForEachStep(func(s *Step) {
					Convey(fmt.Sprintf(`Has correct step: %s`, s.CanonicalName()), func() {
						exp := loadStepProto(t, testCase.name, s.CanonicalName())
						So(s.Proto(), ShouldResemble, exp)
					})
				})

				// Iterate over each generated log and assert that it matches its
				// expectations.
				logs := make([]string, 0, len(cb.logs))
				for k := range cb.logs {
					logs = append(logs, string(k))
				}
				sort.Strings(logs)
				for _, logName := range logs {
					log := cb.logs[types.StreamName(logName)]
					exp := loadLogText(t, testCase.name, logName)
					So(log, ShouldResemble, exp)
				}
			})
		}

		Convey(`Append to a closed State is a no-op.`, func() {
			s.Finish()
			sclone := s
			So(s.Append("asdf"), ShouldBeNil)
			So(s, ShouldResemble, sclone)
		})
	})
}
