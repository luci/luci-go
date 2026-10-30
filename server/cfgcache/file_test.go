// Copyright 2026 The LUCI Authors.
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

package cfgcache

import (
	"testing"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestSingleFile(t *testing.T) {
	t.Parallel()

	t.Run(`default get`, func(t *testing.T) {
		t.Parallel()

		snap := newSnapshot("whatever")
		sf := SingleFile[string]{Path: "foobar${var}"}
		sf.recordInSnapshot(snap, "foobar-resolved", nil)

		assert.That(t, sf.Get(snap), should.Equal(""))
		assert.That(t, sf.ResolvedPath(snap), should.Equal("foobar-resolved"))

		assert.That(t, sf.pattern(), should.Equal("foobar${var}"))
		assert.That(t, sf.isRegex(), should.BeFalse)
	})

	t.Run(`value get`, func(t *testing.T) {
		t.Parallel()

		snap := newSnapshot("whatever")
		sf := SingleFile[string]{Path: "foobar${var}"}
		sf.recordInSnapshot(snap, "foobar-resolved", "banana")

		assert.That(t, sf.Get(snap), should.Equal("banana"))
		assert.That(t, sf.ResolvedPath(snap), should.Equal("foobar-resolved"))
	})

	t.Run(`non-default get`, func(t *testing.T) {
		t.Parallel()

		snap := newSnapshot("whatever")
		sf := SingleFile[string]{Path: "foobar${var}", NewDefault: func() string { return "pants" }}
		sf.recordInSnapshot(snap, "foobar-resolved", nil)

		assert.That(t, sf.Get(snap), should.Equal("pants"))
		assert.That(t, sf.ResolvedPath(snap), should.Equal("foobar-resolved"))
	})
}

func TestRegexFile(t *testing.T) {
	t.Parallel()

	t.Run(`default get`, func(t *testing.T) {
		t.Parallel()

		snap := newSnapshot("whatever")
		sf := RegexFile[string]{PathRegex: ".*"}

		assert.That(t, sf.Get(snap), should.Match(map[string]string(nil)))

		assert.That(t, sf.pattern(), should.Equal(".*"))
		assert.That(t, sf.isRegex(), should.BeTrue)
	})

	t.Run(`value get`, func(t *testing.T) {
		t.Parallel()

		snap := newSnapshot("whatever")
		sf := RegexFile[string]{PathRegex: ".*"}
		sf.recordInSnapshot(snap, "a", "banana")
		sf.recordInSnapshot(snap, "b/c", "pants")

		assert.That(t, sf.Get(snap), should.Match(map[string]string{
			"a":   "banana",
			"b/c": "pants",
		}))
	})
}
