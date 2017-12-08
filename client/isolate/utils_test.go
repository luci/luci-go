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

package isolate

import (
	"errors"
	"io/ioutil"
	"log"
	"os"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestUniqueMergeSortedStrings(t *testing.T) {
	t.Parallel()
	Convey(`Tests the unique merge of sorted of strings.`, t, func() {
		SS := func(s string) []string {
			out := make([]string, 0, len(s))
			for _, c := range s {
				out = append(out, string(c))
			}
			return out
		}
		So(uniqueMergeSortedStrings(SS("acde"), SS("abe")), ShouldResemble, SS("abcde"))
		So(uniqueMergeSortedStrings(SS("abc"), SS("")), ShouldResemble, SS("abc"))
		So(uniqueMergeSortedStrings(
			[]string{"bar", "foo", "test"},
			[]string{"foo", "toss", "xyz"}),
			ShouldResemble, []string{"bar", "foo", "test", "toss", "xyz"})

		// Test degenerate cases (empty and single-element lists)
		So(uniqueMergeSortedStrings(SS(""), SS("")), ShouldResemble, SS(""))
		So(uniqueMergeSortedStrings(SS("x"), SS("")), ShouldResemble, SS("x"))
		So(uniqueMergeSortedStrings(SS(""), SS("x")), ShouldResemble, SS("x"))
	})
}

func TestAssert(t *testing.T) {
	Convey(`Helper function for test assertion.`, t, func() {
		log.SetOutput(ioutil.Discard)
		defer log.SetOutput(os.Stderr)

		wasPanic := func(f func()) (yes bool) {
			defer func() {
				yes = nil != recover()
			}()
			f()
			return
		}
		So(wasPanic(func() { assert(false) }), ShouldBeTrue)
		So(wasPanic(func() { assert(false, "format") }), ShouldBeTrue)
		So(wasPanic(func() { assert(false, "format") }), ShouldBeTrue)
		So(wasPanic(func() { assertNoError(errors.New("error")) }), ShouldBeTrue)
	})
}

// Copy-pasted from Go's lib path/filepath/path_test.go .
//
// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
type RelTests struct {
	root, path, want string
}

var reltests = []RelTests{
	{"a/b", "a/b", "."},
	{"a/b/.", "a/b", "."},
	{"a/b", "a/b/.", "."},
	{"./a/b", "a/b", "."},
	{"a/b", "./a/b", "."},
	{"ab/cd", "ab/cde", "../cde"},
	{"ab/cd", "ab/c", "../c"},
	{"a/b", "a/b/c/d", "c/d"},
	{"a/b", "a/b/../c", "../c"},
	{"a/b/../c", "a/b", "../b"},
	{"a/b/c", "a/c/d", "../../c/d"},
	{"a/b", "c/d", "../../c/d"},
	{"a/b/c/d", "a/b", "../.."},
	{"a/b/c/d", "a/b/", "../.."},
	{"a/b/c/d/", "a/b", "../.."},
	{"a/b/c/d/", "a/b/", "../.."},
	{"../../a/b", "../../a/b/c/d", "c/d"},
	{"/a/b", "/a/b", "."},
	{"/a/b/.", "/a/b", "."},
	{"/a/b", "/a/b/.", "."},
	{"/ab/cd", "/ab/cde", "../cde"},
	{"/ab/cd", "/ab/c", "../c"},
	{"/a/b", "/a/b/c/d", "c/d"},
	{"/a/b", "/a/b/../c", "../c"},
	{"/a/b/../c", "/a/b", "../b"},
	{"/a/b/c", "/a/c/d", "../../c/d"},
	{"/a/b", "/c/d", "../../c/d"},
	{"/a/b/c/d", "/a/b", "../.."},
	{"/a/b/c/d", "/a/b/", "../.."},
	{"/a/b/c/d/", "/a/b", "../.."},
	{"/a/b/c/d/", "/a/b/", "../.."},
	{"/../../a/b", "/../../a/b/c/d", "c/d"},
	{".", "a/b", "a/b"},
	{".", "..", ".."},

	// can't do purely lexically
	{"..", ".", "err"},
	{"..", "a", "err"},
	{"../..", "..", "err"},
	{"a", "/a", "err"},
	{"/a", "a", "err"},
}

func TestPosixRel(t *testing.T) {
	t.Parallel()
	for _, test := range reltests {
		got, err := posixRel(test.root, test.path)
		if test.want == "err" {
			if err == nil {
				t.Errorf("Rel(%q, %q)=%q, want error", test.root, test.path, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("Rel(%q, %q): want %q, got error: %s", test.root, test.path, test.want, err)
		}
		if got != test.want {
			t.Errorf("Rel(%q, %q)=%q, want %q", test.root, test.path, got, test.want)
		}
	}
}
