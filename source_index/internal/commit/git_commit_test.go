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

package commit

import (
	"errors"
	"testing"

	"go.chromium.org/luci/common/proto/git"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

const msgWithNoCommitPosition = `
commit title

commit message body.
`

const msgWithCrCommitPosition = `
commit title

Cr-Commit-Position: refs/heads/main@{#42}
`

const msgWithMultipleCrCommitPositions = `
commit title

Cr-Commit-Position: refs/heads/foo@{#41}
Cr-Commit-Position: refs/heads/main@{#42}
`

const msgWithSVNCommitPosition = `
commit title

git-svn-id: svn://svn.chromium.org/chrome/trunk/src@42 00000000-0000-0000-0000-000000000000
`

const msgWithMixedCommitPositions = `
commit title

Cr-Commit-Position: refs/heads/main@{#42}
git-svn-id: svn://svn.chromium.org/chrome/trunk/src@42 00000000-0000-0000-0000-000000000000
`

const msgWithMixedCommitPositionsSvnFirst = `
commit title

git-svn-id: svn://svn.chromium.org/chrome/trunk/src@42 00000000-0000-0000-0000-000000000000
Cr-Commit-Position: refs/heads/main@{#42}
`

const msgWithInvalidCommitPosition = `
commit title

Cr-Commit-Position: not-valid
`

func TestGitCommit(t *testing.T) {
	ftt.Run("GitCommit", t, func(t *ftt.Test) {
		host := "chromium.googlesource.com"
		repository := "chromium/src"
		commitHash := "2ebba24554e605777702c68d34617ae050024e71"
		commit := &git.Commit{
			Id: commitHash,
		}

		t.Run("NewGitCommit", func(t *ftt.Test) {
			t.Run("valid", func(t *ftt.Test) {
				commit.Message = msgWithNoCommitPosition

				commit, err := NewGitCommit(host, repository, commit)

				assert.Loosely(t, err, should.BeNil)
				assert.That(t, commit.Key(), ShouldMatchKey(Key{
					host:       host,
					commitHash: commitHash,
					repository: repository,
				}))
			})

			t.Run("with invalid commit position", func(t *ftt.Test) {
				commit.Message = msgWithInvalidCommitPosition

				commit, err := NewGitCommit(host, repository, commit)

				assert.Loosely(t, err, should.BeNil)
				assert.That(t, commit.Key(), ShouldMatchKey(Key{
					host:       host,
					commitHash: commitHash,
					repository: repository,
				}))
			})

			t.Run("with invalid commit key", func(t *ftt.Test) {
				commit.Id = "not-a-valid-hash"

				commit, err := NewGitCommit(host, repository, commit)

				assert.That(t, err, should.ErrLike("invalid commit key"))
				assert.That(t, commit, ShouldMatchGitCommit(GitCommit{}))
			})
		})

		t.Run("Position", func(t *ftt.Test) {
			t.Run("no commit position", func(t *ftt.Test) {
				commit.Message = msgWithNoCommitPosition
				c, err := NewGitCommit(host, repository, commit)
				assert.Loosely(t, err, should.BeNil)

				pos, err := c.Position()

				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, pos, should.BeNil)
			})

			t.Run("with Cr-Commit-Position footer", func(t *ftt.Test) {
				commit.Message = msgWithCrCommitPosition
				c, err := NewGitCommit(host, repository, commit)
				assert.Loosely(t, err, should.BeNil)

				pos, err := c.Position()

				assert.Loosely(t, err, should.BeNil)
				assert.That(t, pos, should.Match(&Position{Ref: "refs/heads/main", Number: 42}))
			})

			t.Run("with multiple Cr-Commit-Position footers", func(t *ftt.Test) {
				commit.Message = msgWithMultipleCrCommitPositions
				c, err := NewGitCommit(host, repository, commit)
				assert.Loosely(t, err, should.BeNil)

				pos, err := c.Position()

				assert.Loosely(t, err, should.BeNil)
				assert.That(t, pos, should.Match(&Position{Ref: "refs/heads/main", Number: 42}))
			})

			t.Run("with git-svn-id footer", func(t *ftt.Test) {
				commit.Message = msgWithSVNCommitPosition
				c, err := NewGitCommit(host, repository, commit)
				assert.Loosely(t, err, should.BeNil)

				pos, err := c.Position()

				assert.Loosely(t, err, should.BeNil)
				assert.That(t, pos, should.Match(&Position{Ref: "svn://svn.chromium.org/chrome/trunk/src", Number: 42}))
			})

			t.Run("with mixed footers", func(t *ftt.Test) {
				commit.Message = msgWithMixedCommitPositions
				c, err := NewGitCommit(host, repository, commit)
				assert.Loosely(t, err, should.BeNil)

				pos, err := c.Position()

				assert.Loosely(t, err, should.BeNil)
				assert.That(t, pos, should.Match(&Position{Ref: "refs/heads/main", Number: 42}))
			})

			t.Run("with mixed footers ans git-svn-id first", func(t *ftt.Test) {
				commit.Message = msgWithMixedCommitPositionsSvnFirst
				c, err := NewGitCommit(host, repository, commit)
				assert.Loosely(t, err, should.BeNil)

				pos, err := c.Position()

				assert.Loosely(t, err, should.BeNil)
				assert.That(t, pos, should.Match(&Position{Ref: "refs/heads/main", Number: 42}))
			})

			t.Run("with invalid footer", func(t *ftt.Test) {
				commit.Message = msgWithInvalidCommitPosition
				c, err := NewGitCommit(host, repository, commit)
				assert.Loosely(t, err, should.BeNil)

				pos, err := c.Position()

				assert.That(t, errors.Is(err, ErrInvalidPositionFooter), should.BeTrue)
				assert.Loosely(t, pos, should.BeNil)
			})
		})
	})
}
