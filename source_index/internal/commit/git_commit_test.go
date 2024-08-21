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

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
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
	Convey("GitCommit", t, func() {
		host := "chromium.googlesource.com"
		repository := "chromium/src"
		commitHash := "2ebba24554e605777702c68d34617ae050024e71"
		commit := &git.Commit{
			Id: commitHash,
		}

		Convey("NewGitCommit", func() {
			Convey("valid", func() {
				commit.Message = msgWithNoCommitPosition

				commit, err := NewGitCommit(host, repository, commit)

				So(err, ShouldBeNil)
				So(commit.Key(), ShouldResemble, Key{
					host:       host,
					commitHash: commitHash,
					repository: repository,
				})
			})

			Convey("with invalid commit position", func() {
				commit.Message = msgWithInvalidCommitPosition

				commit, err := NewGitCommit(host, repository, commit)

				So(err, ShouldBeNil)
				So(commit.Key(), ShouldResemble, Key{
					host:       host,
					commitHash: commitHash,
					repository: repository,
				})
			})

			Convey("with invalid commit key", func() {
				commit.Id = "not-a-valid-hash"

				commit, err := NewGitCommit(host, repository, commit)

				So(err, ShouldErrLike, "invalid commit key")
				So(commit, ShouldResemble, GitCommit{})
			})
		})

		Convey("Position", func() {
			Convey("no commit position", func() {
				commit.Message = msgWithNoCommitPosition
				c, err := NewGitCommit(host, repository, commit)
				So(err, ShouldBeNil)

				pos, err := c.Position()

				So(err, ShouldBeNil)
				So(pos, ShouldBeNil)
			})

			Convey("with Cr-Commit-Position footer", func() {
				commit.Message = msgWithCrCommitPosition
				c, err := NewGitCommit(host, repository, commit)
				So(err, ShouldBeNil)

				pos, err := c.Position()

				So(err, ShouldBeNil)
				So(pos, ShouldResemble, &Position{Ref: "refs/heads/main", Number: 42})
			})

			Convey("with multiple Cr-Commit-Position footers", func() {
				commit.Message = msgWithMultipleCrCommitPositions
				c, err := NewGitCommit(host, repository, commit)
				So(err, ShouldBeNil)

				pos, err := c.Position()

				So(err, ShouldBeNil)
				So(pos, ShouldResemble, &Position{Ref: "refs/heads/main", Number: 42})
			})

			Convey("with git-svn-id footer", func() {
				commit.Message = msgWithSVNCommitPosition
				c, err := NewGitCommit(host, repository, commit)
				So(err, ShouldBeNil)

				pos, err := c.Position()

				So(err, ShouldBeNil)
				So(pos, ShouldResemble, &Position{Ref: "svn://svn.chromium.org/chrome/trunk/src", Number: 42})
			})

			Convey("with mixed footers", func() {
				commit.Message = msgWithMixedCommitPositions
				c, err := NewGitCommit(host, repository, commit)
				So(err, ShouldBeNil)

				pos, err := c.Position()

				So(err, ShouldBeNil)
				So(pos, ShouldResemble, &Position{Ref: "refs/heads/main", Number: 42})
			})

			Convey("with mixed footers ans git-svn-id first", func() {
				commit.Message = msgWithMixedCommitPositionsSvnFirst
				c, err := NewGitCommit(host, repository, commit)
				So(err, ShouldBeNil)

				pos, err := c.Position()

				So(err, ShouldBeNil)
				So(pos, ShouldResemble, &Position{Ref: "refs/heads/main", Number: 42})
			})

			Convey("with invalid footer", func() {
				commit.Message = msgWithInvalidCommitPosition
				c, err := NewGitCommit(host, repository, commit)
				So(err, ShouldBeNil)

				pos, err := c.Position()

				So(errors.Is(err, ErrInvalidPositionFooter), ShouldBeTrue)
				So(pos, ShouldBeNil)
			})
		})
	})
}
