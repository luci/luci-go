// Copyright 2015 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package annotation

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/golang/protobuf/proto"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/logdog/common/types"
	annopb "go.chromium.org/luci/luciexe/legacy/annotee/proto"
)

const testDataDir = "test_data"
const testExpDir = "test_expectations"

var generate = flag.Bool("annotee.generate", false, "If true, regenerate expectations from source.")

type testCase struct {
	name string
	exe  *Execution
}

func (tc *testCase) state(startTime time.Time) *State {
	cb := testCallbacks{
		closed:   map[*Step]struct{}{},
		logs:     map[types.StreamName][]string{},
		logsOpen: map[types.StreamName]struct{}{},
	}
	return &State{
		LogNameBase: types.StreamName("base"),
		Callbacks:   &cb,
		Clock:       testclock.New(startTime),
		Execution:   tc.exe,
	}
}

func (tc *testCase) generate(t *testing.T, startTime time.Time, touched stringset.Set) error {
	st := tc.state(startTime)
	p, err := playAnnotationScript(t, tc.name, st)
	if err != nil {
		return err
	}
	touched.Add(p)
	st.Finish()

	// Write out generated protos.
	merr := errors.MultiError(nil)

	step := st.RootStep()
	p, err = writeStepProto(tc.name, step)
	if err != nil {
		merr = append(merr, fmt.Errorf("Failed to write step proto for %q::%q: %v", tc.name, step.LogNameBase, err))
	}
	touched.Add(p)

	// Write out generated logs.
	cb := st.Callbacks.(*testCallbacks)
	for logName, lines := range cb.logs {
		p, err := writeLogText(tc.name, string(logName), lines)
		if err != nil {
			merr = append(merr, fmt.Errorf("Failed to write log text for %q::%q: %v", tc.name, logName, err))
		}
		touched.Add(p)
	}

	if merr != nil {
		return merr
	}
	return nil
}

func normalize(s string) string {
	return strings.Map(func(r rune) rune {
		if r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)) {
			return r
		}
		return '_'
	}, s)
}

func superfluous(touched stringset.Set) ([]string, error) {
	var paths []string

	files, err := os.ReadDir(testExpDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory %q: %v", testExpDir, err)
	}

	for _, f := range files {
		if f.IsDir() {
			continue
		}

		path := filepath.Join(testExpDir, f.Name())
		if !touched.Has(path) {
			paths = append(paths, path)
		}
	}
	return paths, nil
}

// playAnnotationScript loads named annotation script and plays it
// through the supplied State line-by-line. Returns path to the annotation
// script.
//
// Empty lines and lines beginning with "#" are ignored. Preceding whitespace
// is discarded.
func playAnnotationScript(t testing.TB, name string, st *State) (string, error) {
	t.Helper()
	tc := st.Clock.(testclock.TestClock)

	path := filepath.Join(testDataDir, fmt.Sprintf("%s.annotations.txt", normalize(name)))
	f, err := os.Open(path)
	if err != nil {
		t.Errorf("Failed to open annotations source [%s]: %v", path, err)
		return "", err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	var nextErr string
	for lineNo := 1; scanner.Scan(); lineNo++ {
		// Trim, discard empty lines and comment lines.
		line := strings.TrimLeftFunc(scanner.Text(), unicode.IsSpace)
		if len(line) == 0 || strings.HasPrefix(line, "#") {
			continue
		}

		switch {
		case line == "+time":
			tc.Add(1 * time.Second)

		case strings.HasPrefix(line, "+error"):
			nextErr = strings.SplitN(line, " ", 2)[1]

		default:
			// Annotation.
			err := st.Append(line)
			if nextErr != "" {
				expectedErr := nextErr
				nextErr = ""

				if err == nil {
					return "", fmt.Errorf("line %d: expected error, but didn't encounter it: %q", lineNo, expectedErr)
				}
				if !strings.Contains(err.Error(), expectedErr) {
					return "", fmt.Errorf("line %d: expected error %q, but got: %v", lineNo, expectedErr, err)
				}
			} else if err != nil {
				return "", err
			}
		}
	}

	return path, nil
}

func loadStepProto(t testing.TB, test string, s *Step) *annopb.Step {
	path := filepath.Join(testExpDir, fmt.Sprintf("%s_%s.proto.txt", normalize(test), normalize(string(s.LogNameBase))))
	data, err := os.ReadFile(path)
	if err != nil {
		t.Errorf("Failed to read annopb.Step proto [%s]: %v", path, err)
		return nil
	}

	st := annopb.Step{}
	if err := proto.UnmarshalText(string(data), &st); err != nil {
		t.Errorf("Failed to Unmarshal annopb.Step proto [%s]: %v", path, err)
		return nil
	}
	return &st
}

func writeStepProto(test string, s *Step) (string, error) {
	path := filepath.Join(testExpDir, fmt.Sprintf("%s_%s.proto.txt", normalize(test), normalize(string(s.LogNameBase))))
	return path, os.WriteFile(path, []byte(proto.MarshalTextString(s.Proto())), 0644)
}

func loadLogText(t testing.TB, test, name string) []string {
	path := filepath.Join(testExpDir, fmt.Sprintf("%s_%s.txt", normalize(test), normalize(name)))
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

func writeLogText(test, name string, text []string) (string, error) {
	path := filepath.Join(testExpDir, fmt.Sprintf("%s_%s.txt", normalize(test), normalize(name)))
	return path, os.WriteFile(path, []byte(strings.Join(text, "\n")), 0644)
}

// testCallbacks implements the Callbacks interface, retaining all callback
// data in memory.
type testCallbacks struct {
	// closed is the set of steps that have been closed.
	closed map[*Step]struct{}

	// logs is the content of emitted annotation logs, keyed on stream name.
	logs map[types.StreamName][]string
	// logsOpen tracks whether a given annotation log is open.
	logsOpen map[types.StreamName]struct{}
}

func (tc *testCallbacks) StepClosed(s *Step) {
	tc.closed[s] = struct{}{}
}

func (tc *testCallbacks) StepLogLine(s *Step, n types.StreamName, label, line string) {
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

func (tc *testCallbacks) Updated(s *Step, ut UpdateType) {}

func TestState(t *testing.T) {
	t.Parallel()

	startTime := time.Date(2015, 1, 1, 0, 0, 0, 0, time.UTC)
	testCases := []testCase{
		{"default", &Execution{
			Name:    "testcommand",
			Command: []string{"testcommand", "foo", "bar"},
			Dir:     "/path/to/dir",
			Env: map[string]string{
				"FOO": "BAR",
				"BAZ": "QUX",
			},
		}},
		{"timestamps", nil},
		{"coverage", nil},
		{"nested", nil},
		{"legacy", nil},
	}

	if *generate {
		touched := stringset.New(0)
		for _, tc := range testCases {
			if err := tc.generate(t, startTime, touched); err != nil {
				t.Fatalf("Failed to generate %q: %v\n", tc.name, err)
			}
		}

		paths, err := superfluous(touched)
		if err != nil {
			if merr, ok := err.(errors.MultiError); ok {
				for i, ierr := range merr {
					t.Logf("Error #%d: %s", i, ierr)
				}
			}
			t.Fatalf("Superfluous test data: %v", err)
		}
		for _, path := range paths {
			t.Log("Removing superfluous test data:", path)
			os.Remove(path)
		}
		return
	}

	ftt.Run(`A testing annotation State`, t, func(t *ftt.Test) {
		for _, testCase := range testCases {
			st := testCase.state(startTime)

			t.Run(fmt.Sprintf(`Correctly loads/generates for %q test case.`, testCase.name), func(t *ftt.Test) {

				_, err := playAnnotationScript(t, testCase.name, st)
				assert.Loosely(t, err, should.BeNil)

				// Iterate through generated streams and validate.
				st.Finish()

				// All log streams should be closed.
				cb := st.Callbacks.(*testCallbacks)
				assert.Loosely(t, cb.logsOpen, should.Resemble(map[types.StreamName]struct{}{}))

				// Iterate over each generated stream and assert that it matches its
				// expectation. Do it deterministically so failures aren't frustrating
				// to reproduce.
				t.Run(`Has correct Step value`, func(t *ftt.Test) {
					rootStep := st.RootStep()

					exp := loadStepProto(t, testCase.name, rootStep)
					assert.Loosely(t, rootStep.Proto(), should.Resemble(exp))
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
					assert.Loosely(t, log, should.Resemble(exp))
				}
			})
		}

		t.Run(`Append to a closed State is a no-op.`, func(t *ftt.Test) {
			st := testCases[0].state(startTime)
			st.Finish()
			sclone := st
			assert.Loosely(t, st.Append("asdf"), should.BeNil)
			assert.Loosely(t, st, should.Equal(sclone))
		})
	})
}
