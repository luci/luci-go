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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestUniqueMergeSortedStrings(t *testing.T) {
	t.Parallel()
	ftt.Run(`Tests the unique merge of sorted of strings.`, t, func(t *ftt.Test) {
		SS := func(s string) []string {
			out := make([]string, 0, len(s))
			for _, c := range s {
				out = append(out, string(c))
			}
			return out
		}
		assert.Loosely(t, uniqueMergeSortedStrings(SS("acde"), SS("abe")), should.Resemble(SS("abcde")))
		assert.Loosely(t, uniqueMergeSortedStrings(SS("abc"), SS("")), should.Resemble(SS("abc")))
		assert.Loosely(t, uniqueMergeSortedStrings(
			[]string{"bar", "foo", "test"},
			[]string{"foo", "toss", "xyz"}),
			should.Resemble([]string{"bar", "foo", "test", "toss", "xyz"}))

		// Test degenerate cases (empty and single-element lists)
		assert.Loosely(t, uniqueMergeSortedStrings(SS(""), SS("")), should.Resemble(SS("")))
		assert.Loosely(t, uniqueMergeSortedStrings(SS("x"), SS("")), should.Resemble(SS("x")))
		assert.Loosely(t, uniqueMergeSortedStrings(SS(""), SS("x")), should.Resemble(SS("x")))
	})
}

func TestMust(t *testing.T) {
	ftt.Run(`Helper function for test assertion.`, t, func(t *ftt.Test) {
		log.SetOutput(io.Discard)
		defer log.SetOutput(os.Stderr)

		wasPanic := func(f func()) (yes bool) {
			defer func() {
				yes = nil != recover()
			}()
			f()
			return
		}
		assert.Loosely(t, wasPanic(func() { must(false) }), should.BeTrue)
		assert.Loosely(t, wasPanic(func() { must(false, "format") }), should.BeTrue)
		assert.Loosely(t, wasPanic(func() { must(false, "format") }), should.BeTrue)
	})
}
