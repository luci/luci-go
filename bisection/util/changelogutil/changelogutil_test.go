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

package changelogutil

import (
	"context"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/bisection/internal/gitiles"
	"go.chromium.org/luci/bisection/model"
)

func TestParseRollURL(t *testing.T) {
	t.Parallel()
	ftt.Run("ParseRollURL", t, func(t *ftt.Test) {
		t.Run("standard gitiles url with git suffix", func(t *ftt.Test) {
			msg := "Roll src/third_party/skia/ a..b\n\nhttps://skia.googlesource.com/skia.git/+log/c1d2e3f4f..a5b6c7d8e"
			host, project, start, end, ok := ParseRollURL(msg)
			assert.Loosely(t, ok, should.BeTrue)
			assert.Loosely(t, host, should.Equal("skia.googlesource.com"))
			assert.Loosely(t, project, should.Equal("skia"))
			assert.Loosely(t, start, should.Equal("c1d2e3f4f"))
			assert.Loosely(t, end, should.Equal("a5b6c7d8e"))
		})

		t.Run("standard gitiles url without git suffix", func(t *ftt.Test) {
			msg := "Roll v8: https://chromium.googlesource.com/v8/v8/+log/1234567..890abcd"
			host, project, start, end, ok := ParseRollURL(msg)
			assert.Loosely(t, ok, should.BeTrue)
			assert.Loosely(t, host, should.Equal("chromium.googlesource.com"))
			assert.Loosely(t, project, should.Equal("v8/v8"))
			assert.Loosely(t, start, should.Equal("1234567"))
			assert.Loosely(t, end, should.Equal("890abcd"))
		})

		t.Run("real-life videolan/dav1d roll URL", func(t *ftt.Test) {
			msg := "Roll src/third_party/dav1d/libdav1d/ 1718ff9ad..62501cc7d (3 commits)\n\nhttps://chromium.googlesource.com/external/github.com/videolan/dav1d.git/+log/1718ff9aded9..62501cc7db37"
			host, project, start, end, ok := ParseRollURL(msg)
			assert.Loosely(t, ok, should.BeTrue)
			assert.Loosely(t, host, should.Equal("chromium.googlesource.com"))
			assert.Loosely(t, project, should.Equal("external/github.com/videolan/dav1d"))
			assert.Loosely(t, start, should.Equal("1718ff9aded9"))
			assert.Loosely(t, end, should.Equal("62501cc7db37"))
		})

		t.Run("no roll url", func(t *ftt.Test) {
			msg := "Fix compiler warning\n\nReviewed-on: https://chromium-review.googlesource.com/123"
			_, _, _, _, ok := ParseRollURL(msg)
			assert.Loosely(t, ok, should.BeFalse)
		})
	})
}

func TestParseRollPrefix(t *testing.T) {
	t.Parallel()
	ftt.Run("ParseRollPrefix", t, func(t *ftt.Test) {
		t.Run("standard path with trailing slash", func(t *ftt.Test) {
			msg := "Roll src/third_party/skia/ a..b"
			assert.Loosely(t, ParseRollPrefix(msg), should.Equal("src/third_party/skia/"))
		})

		t.Run("standard path without trailing slash", func(t *ftt.Test) {
			msg := "Roll third_party/catapult a..b"
			assert.Loosely(t, ParseRollPrefix(msg), should.Equal("third_party/catapult/"))
		})

		t.Run("roll with colon", func(t *ftt.Test) {
			msg := "Roll recipe_engine: 12 commits"
			assert.Loosely(t, ParseRollPrefix(msg), should.Equal("recipe_engine/"))
		})

		t.Run("real-life dav1d roll prefix", func(t *ftt.Test) {
			msg := "Roll src/third_party/dav1d/libdav1d/ 1718ff9ad..62501cc7d (3 commits)"
			assert.Loosely(t, ParseRollPrefix(msg), should.Equal("src/third_party/dav1d/libdav1d/"))
		})

		t.Run("no path", func(t *ftt.Test) {
			msg := "Some random commit message"
			assert.Loosely(t, ParseRollPrefix(msg), should.Equal(""))
		})
	})
}

func TestExpandChangeLogs(t *testing.T) {
	t.Parallel()
	ftt.Run("ExpandChangeLogs", t, func(t *ftt.Test) {
		c := context.Background()

		t.Run("No roll CLs in blamelist", func(t *ftt.Test) {
			changelogs := []*model.ChangeLog{
				{
					Commit:  "commit1",
					Message: "Normal commit 1\n",
				},
				{
					Commit:  "commit2",
					Message: "Normal commit 2\n",
				},
			}
			res, err := ExpandChangeLogs(c, changelogs, "https://chromium.googlesource.com/chromium/src")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(res), should.Equal(2))
			assert.Loosely(t, res[0].ChangeLog, should.Match(changelogs[0]))
			assert.Loosely(t, res[1].ChangeLog, should.Match(changelogs[1]))
		})

		t.Run("With roll CL, successfully expands", func(t *ftt.Test) {
			gitilesData := map[string]string{
				"https://skia.googlesource.com/skia/+log/c1d2e3f4f..a5b6c7d8e": `)]}'
{
	"log": [
		{
			"commit": "subcommit1",
			"message": "Skia change 1\n",
			"tree_diff": [
				{
					"type": "modify",
					"old_path": "src/core/SkCanvas.cpp",
					"new_path": "src/core/SkCanvas.cpp"
				},
				{
					"type": "add",
					"old_path": "/dev/null",
					"new_path": "src/core/SkNewFile.cpp"
				},
				{
					"type": "delete",
					"old_path": "src/core/SkDeletedFile.cpp",
					"new_path": "/dev/null"
				}
			]
		}
	]
}`,
			}
			c = gitiles.MockedGitilesClientContext(c, gitilesData)

			changelogs := []*model.ChangeLog{
				{
					Commit:  "commit1",
					Message: "Normal commit 1\n",
				},
				{
					Commit:  "rollcommit",
					Message: "Roll src/third_party/skia/ c1d2e3f4f..a5b6c7d8e\n\nhttps://skia.googlesource.com/skia.git/+log/c1d2e3f4f..a5b6c7d8e",
				},
			}

			res, err := ExpandChangeLogs(c, changelogs, "https://chromium.googlesource.com/chromium/src")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(res), should.Equal(3))
			assert.Loosely(t, res[0].Commit, should.Equal("commit1"))
			assert.Loosely(t, res[1].Commit, should.Equal("rollcommit"))
			assert.Loosely(t, res[2].Commit, should.Equal("subcommit1"))
			assert.Loosely(t, res[2].RepoURL, should.Equal("https://skia.googlesource.com/skia"))
			assert.Loosely(t, res[2].ChangeLogDiffs[0].NewPath, should.Equal("src/third_party/skia/src/core/SkCanvas.cpp"))
			assert.Loosely(t, res[2].ChangeLogDiffs[1].OldPath, should.Equal("/dev/null"))
			assert.Loosely(t, res[2].ChangeLogDiffs[1].NewPath, should.Equal("src/third_party/skia/src/core/SkNewFile.cpp"))
			assert.Loosely(t, res[2].ChangeLogDiffs[2].OldPath, should.Equal("src/third_party/skia/src/core/SkDeletedFile.cpp"))
			assert.Loosely(t, res[2].ChangeLogDiffs[2].NewPath, should.Equal("/dev/null"))
		})

		t.Run("With revert CL, skips expansion", func(t *ftt.Test) {
			changelogs := []*model.ChangeLog{
				{
					Commit:  "revertcommit",
					Message: "Revert \"Roll src/third_party/skia/\"\n\nThis reverts commit rollcommit.\n\nOriginal description:\n> Roll src/third_party/skia/ c1d2e3f4f..a5b6c7d8e\n> https://skia.googlesource.com/skia.git/+log/c1d2e3f4f..a5b6c7d8e",
				},
			}
			res, err := ExpandChangeLogs(c, changelogs, "https://chromium.googlesource.com/chromium/src")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(res), should.Equal(1))
			assert.Loosely(t, res[0].Commit, should.Equal("revertcommit"))
		})

		t.Run("Exceeds recursion depth limit, stops expanding", func(t *ftt.Test) {
			gitilesData := map[string]string{
				"https://repo0.googlesource.com/proj0/+log/0000000..1111111": `)]}'
{
	"log": [
		{
			"commit": "roll1",
			"message": "Roll repo1 1111111..2222222\n\nhttps://repo1.googlesource.com/proj1/+log/1111111..2222222"
		}
	]
}`,
				"https://repo1.googlesource.com/proj1/+log/1111111..2222222": `)]}'
{
	"log": [
		{
			"commit": "roll2",
			"message": "Roll repo2 2222222..3333333\n\nhttps://repo2.googlesource.com/proj2/+log/2222222..3333333"
		}
	]
}`,
				"https://repo2.googlesource.com/proj2/+log/2222222..3333333": `)]}'
{
	"log": [
		{
			"commit": "roll3",
			"message": "Roll repo3 3333333..4444444\n\nhttps://repo3.googlesource.com/proj3/+log/3333333..4444444"
		}
	]
}`,
				"https://repo3.googlesource.com/proj3/+log/3333333..4444444": `)]}'
{
	"log": [
		{
			"commit": "roll4",
			"message": "Roll repo4 4444444..5555555\n\nhttps://repo4.googlesource.com/proj4/+log/4444444..5555555"
		}
	]
}`,
				"https://repo4.googlesource.com/proj4/+log/4444444..5555555": `)]}'
{
	"log": [
		{
			"commit": "roll5",
			"message": "Roll repo5 5555555..6666666\n\nhttps://repo5.googlesource.com/proj5/+log/5555555..6666666"
		}
	]
}`,
			}
			c = gitiles.MockedGitilesClientContext(c, gitilesData)

			changelogs := []*model.ChangeLog{
				{
					Commit:  "roll0",
					Message: "Roll repo0 0000000..1111111\n\nhttps://repo0.googlesource.com/proj0/+log/0000000..1111111",
				},
			}

			res, err := ExpandChangeLogs(c, changelogs, "https://chromium.googlesource.com/chromium/src")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(res), should.Equal(6))
			assert.Loosely(t, res[0].Commit, should.Equal("roll0"))
			assert.Loosely(t, res[1].Commit, should.Equal("roll1"))
			assert.Loosely(t, res[2].Commit, should.Equal("roll2"))
			assert.Loosely(t, res[3].Commit, should.Equal("roll3"))
			assert.Loosely(t, res[4].Commit, should.Equal("roll4"))
			assert.Loosely(t, res[5].Commit, should.Equal("roll5"))
		})
	})
}

func TestParseRepoURL(t *testing.T) {
	t.Parallel()
	ftt.Run("ParseRepoURL", t, func(t *ftt.Test) {
		t.Run("valid url", func(t *ftt.Test) {
			host, project, err := ParseRepoURL("https://chromium.googlesource.com/chromium/src")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, host, should.Equal("chromium.googlesource.com"))
			assert.Loosely(t, project, should.Equal("chromium/src"))
		})

		t.Run("invalid url", func(t *ftt.Test) {
			_, _, err := ParseRepoURL("ftp://invalid-url")
			assert.Loosely(t, err, should.NotBeNil)
		})
	})
}

func TestIsRevert(t *testing.T) {
	t.Parallel()
	ftt.Run("IsRevert", t, func(t *ftt.Test) {
		assert.Loosely(t, IsRevert("Revert \"Roll Skia\""), should.BeTrue)
		assert.Loosely(t, IsRevert("Revert: Roll Skia"), should.BeTrue)
		assert.Loosely(t, IsRevert("Roll Skia"), should.BeFalse)
		assert.Loosely(t, IsRevert("Fix bug"), should.BeFalse)
	})
}
