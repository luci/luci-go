// Copyright 2023 The LUCI Authors.
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

package pbutil

import (
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestValidate(t *testing.T) {
	t.Parallel()
	Convey(`ValidateGitilesCommit`, t, func() {
		commit := &pb.GitilesCommit{
			Host:       "chromium.googlesource.com",
			Project:    "chromium/src",
			Ref:        "refs/heads/branch",
			CommitHash: "123456789012345678901234567890abcdefabcd",
			Position:   1,
		}
		Convey(`Valid`, func() {
			So(ValidateGitilesCommit(commit), ShouldBeNil)
		})
		Convey(`Nil`, func() {
			So(ValidateGitilesCommit(nil), ShouldErrLike, `unspecified`)
		})
		Convey(`Host`, func() {
			Convey(`Missing`, func() {
				commit.Host = ""
				So(ValidateGitilesCommit(commit), ShouldErrLike, `host: unspecified`)
			})
			Convey(`Invalid format`, func() {
				commit.Host = "https://somehost.com"
				So(ValidateGitilesCommit(commit), ShouldErrLike, `host: does not match`)
			})
			Convey(`Too long`, func() {
				commit.Host = strings.Repeat("a", hostnameMaxLength+1)
				So(ValidateGitilesCommit(commit), ShouldErrLike, `host: exceeds `, ` characters`)
			})
		})
		Convey(`Project`, func() {
			Convey(`Missing`, func() {
				commit.Project = ""
				So(ValidateGitilesCommit(commit), ShouldErrLike, `project: unspecified`)
			})
			Convey(`Too long`, func() {
				commit.Project = strings.Repeat("a", 256)
				So(ValidateGitilesCommit(commit), ShouldErrLike, `project: exceeds 255 characters`)
			})
		})
		Convey(`Refs`, func() {
			Convey(`Missing`, func() {
				commit.Ref = ""
				So(ValidateGitilesCommit(commit), ShouldErrLike, `ref: unspecified`)
			})
			Convey(`Invalid`, func() {
				commit.Ref = "main"
				So(ValidateGitilesCommit(commit), ShouldErrLike, `ref: does not match refs/.*`)
			})
			Convey(`Too long`, func() {
				commit.Ref = "refs/" + strings.Repeat("a", 252)
				So(ValidateGitilesCommit(commit), ShouldErrLike, `ref: exceeds 255 characters`)
			})
		})
		Convey(`Commit Hash`, func() {
			Convey(`Missing`, func() {
				commit.CommitHash = ""
				So(ValidateGitilesCommit(commit), ShouldErrLike, `commit_hash: unspecified`)
			})
			Convey(`Invalid (too long)`, func() {
				commit.CommitHash = strings.Repeat("a", 41)
				So(ValidateGitilesCommit(commit), ShouldErrLike, `commit_hash: does not match "^[a-f0-9]{40}$"`)
			})
			Convey(`Invalid (too short)`, func() {
				commit.CommitHash = strings.Repeat("a", 39)
				So(ValidateGitilesCommit(commit), ShouldErrLike, `commit_hash: does not match "^[a-f0-9]{40}$"`)
			})
			Convey(`Invalid (upper case)`, func() {
				commit.CommitHash = "123456789012345678901234567890ABCDEFABCD"
				So(ValidateGitilesCommit(commit), ShouldErrLike, `commit_hash: does not match "^[a-f0-9]{40}$"`)
			})
		})
		Convey(`Position`, func() {
			Convey(`Missing`, func() {
				commit.Position = 0
				So(ValidateGitilesCommit(commit), ShouldErrLike, `position: unspecified`)
			})
		})
	})
	Convey(`ValidateGerritChange`, t, func() {
		change := &pb.GerritChange{
			Host:     "chromium-review.googlesource.com",
			Project:  "chromium/src",
			Change:   12345,
			Patchset: 1,
		}
		Convey(`Valid`, func() {
			So(ValidateGerritChange(change), ShouldBeNil)
		})
		Convey(`Nil`, func() {
			So(ValidateGerritChange(nil), ShouldErrLike, `unspecified`)
		})
		Convey(`Host`, func() {
			Convey(`Missing`, func() {
				change.Host = ""
				So(ValidateGerritChange(change), ShouldErrLike, `host: unspecified`)
			})
			Convey(`Invalid format`, func() {
				change.Host = "https://somehost.com"
				So(ValidateGerritChange(change), ShouldErrLike, `host: does not match`)
			})
			Convey(`Too long`, func() {
				change.Host = strings.Repeat("a", hostnameMaxLength+1)
				So(ValidateGerritChange(change), ShouldErrLike, `host: exceeds `, ` characters`)
			})
		})
		Convey(`Project`, func() {
			Convey(`Missing`, func() {
				change.Project = ""
				So(ValidateGerritChange(change), ShouldErrLike, `project: unspecified`)
			})
			Convey(`Too long`, func() {
				change.Project = strings.Repeat("a", 256)
				So(ValidateGerritChange(change), ShouldErrLike, `project: exceeds 255 characters`)
			})
		})
		Convey(`Change`, func() {
			Convey(`Missing`, func() {
				change.Change = 0
				So(ValidateGerritChange(change), ShouldErrLike, `change: unspecified`)
			})
			Convey(`Invalid`, func() {
				change.Change = -1
				So(ValidateGerritChange(change), ShouldErrLike, `change: cannot be negative`)
			})
		})
		Convey(`Patchset`, func() {
			Convey(`Missing`, func() {
				change.Patchset = 0
				So(ValidateGerritChange(change), ShouldErrLike, `patchset: unspecified`)
			})
			Convey(`Invalid`, func() {
				change.Patchset = -1
				So(ValidateGerritChange(change), ShouldErrLike, `patchset: cannot be negative`)
			})
		})
	})
}
