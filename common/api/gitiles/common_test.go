// Copyright 2017 The LUCI Authors.
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

package gitiles

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestRepoURL(t *testing.T) {
	t.Parallel()
	ftt.Run("Malformed", t, func(t *ftt.Test) {
		f := func(arg string) {
			assert.Loosely(t, ValidateRepoURL(arg), should.NotBeNil)
			_, err := NormalizeRepoURL(arg, true)
			assert.Loosely(t, err, should.NotBeNil)
		}

		f("wtf/\\is\this")
		f("https://example.com/repo.git")
		f("http://bad-protocol.googlesource.com/repo.git")
		f("https://a.googlesource.com")
		f("https://a.googlesource.com/")
		f("a.googlesource.com/no-protocol.git")
		f("https://a.googlesource.com/no-protocol#fragment")
	})

	ftt.Run("OK", t, func(t *ftt.Test) {
		f := func(arg, exp string) {
			assert.Loosely(t, ValidateRepoURL(arg), should.BeNil)
			act, err := NormalizeRepoURL(arg, true)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, act.String(), should.Equal(exp))
		}

		f("https://chromium.googlesource.com/repo.git",
			"https://chromium.googlesource.com/a/repo")
		f("https://chromium.googlesource.com/repo/",
			"https://chromium.googlesource.com/a/repo")
		f("https://chromium.googlesource.com/a/repo",
			"https://chromium.googlesource.com/a/repo")
		f("https://chromium.googlesource.com/parent/repo.git/",
			"https://chromium.googlesource.com/a/parent/repo")
	})
}
