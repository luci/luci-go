// Copyright 2025 The LUCI Authors.
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

package internal

import (
	"testing"

	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestRelPath(t *testing.T) {
	t.Parallel()

	cases := []struct {
		base, target, rel string
	}{
		{".", "a/b/c", "a/b/c"},
		{"a/b/c", ".", "../../.."},
		{".", ".", "."},
		{"a/b/c", "a/b/c", "."},
		{"a/b/c", "a/b/c/d/e", "d/e"},
		{"a/b/c/d/e", "a/b/c", "../.."},
		{"a/b/c/d/e", "a/b/c/f/g", "../../f/g"},

		// Cleans the path.
		{"./a/./b/c", "a/b/c/d/../.", "."},
		{"a/b/c/../../../", "a/b/c", "a/b/c"},
	}

	for _, cs := range cases {
		rel, err := RelRepoPath(cs.base, cs.target)
		assert.NoErr(t, err)
		assert.That(t, rel, should.Equal(cs.rel), truth.Explain("rel(%q, %q)", cs.base, cs.target))
	}

	_, err := RelRepoPath("a/../..", "a/b/c")
	assert.That(t, err, should.ErrLike("goes outside of the repo root"))
	_, err = RelRepoPath("a/b/c", "a/../..")
	assert.That(t, err, should.ErrLike("goes outside of the repo root"))
}
