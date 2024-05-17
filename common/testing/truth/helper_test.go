// Copyright 2024 The LUCI Authors.
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

package truth_test

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"go.chromium.org/luci/common/testing/truth"
)

// fakeTB is just a simple MinimalTestingTB implementation for the example. In
// a real test, this would be *testing.T, *testing.B, etc.
type fakeTB struct {
	testing.TB // panic all unimplemented methods

	// true after first Log call
	firstLog bool
}

var _ testing.TB = (*fakeTB)(nil)

// This is either 'space' (U+0020) or 'non-breaking space' (U+00a0).
var spaceRE = regexp.MustCompile("[  ]")

func (*fakeTB) Helper() {}
func (f *fakeTB) Log(args ...any) {
	if !f.firstLog {
		fmt.Println("--- FAIL: FakeTestName (0.00s)")
		f.firstLog = true
	}

	// HACK - cmp.Diff intentionally has unstable output because they don't want
	// folks to overindex on the particular rendering choices of some version of
	// cmp.Diff. However, we want to have an example test below which shows off
	// SmartCmpDiff. To that end, we accept the possibility that this test could
	// break if cmp.Diff changes, but it's easy enough to update the Output block
	// in that case (and it's pinned via go.mod, so this won't be a surprise
	// breakage).
	fixedArgs := make([]string, 1+len(args))
	fixedArgs[0] = "    filename.go:NN:"
	for i, arg := range args {
		if s, ok := arg.(string); ok {
			fixedArgs[i+1] = spaceRE.ReplaceAllString(s, " ")
		} else {
			panic(fmt.Errorf("unexpected argument to Log: %v[%d]: %q", args, i, arg))
		}
	}
	indent := strings.Repeat(" ", 8)
	lines := strings.Split(strings.Join(fixedArgs, " "), "\n")
	for i := 1; i < len(lines); i++ {
		lines[i] = indent + lines[i]
	}
	fmt.Println(strings.Join(lines, "\n"))
}
func (*fakeTB) Fail()    {}
func (*fakeTB) FailNow() {}

func disableColorization() func() {
	old := truth.Colorize
	truth.Colorize = false
	return func() { truth.Colorize = old }
}
func disableVerbosity() func() {
	old := truth.Verbose
	truth.Verbose = false
	return func() { truth.Verbose = old }
}
func disableFullpath() func() {
	old := truth.FullSourceContextFilenames
	truth.FullSourceContextFilenames = false
	return func() { truth.FullSourceContextFilenames = old }
}
