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
	"io"
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
		log.SetOutput(io.Discard)
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
	})
}
