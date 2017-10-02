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
	"bytes"
	"strings"
	"testing"

	"github.com/golang/protobuf/ptypes/timestamp"
	. "github.com/smartystreets/goconvey/convey"

	"go.chromium.org/luci/common/proto/git"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestUserProto(t *testing.T) {
	t.Parallel()

	Convey(`Test User.Proto`, t, func() {
		u := &User{
			Name:  "Some name",
			Email: "some.name@example.com",
			Time:  "Wed Mar 09 03:46:18 2016",
		}

		Convey(`basic`, func() {
			uPB, err := u.Proto()
			So(err, ShouldBeNil)
			So(uPB, ShouldResemble, &git.Commit_User{
				Name:  "Some name",
				Email: "some.name@example.com",
				Time: &timestamp.Timestamp{
					Seconds: 1457495178,
				},
			})
		})

		Convey(`empty timestamp`, func() {
			u.Time = ""
			uPB, err := u.Proto()
			So(err, ShouldBeNil)
			So(uPB, ShouldResemble, &git.Commit_User{
				Name:  "Some name",
				Email: "some.name@example.com",
			})
		})

		Convey(`bad timestamp`, func() {
			u.Time = "nerdz"
			_, err := u.Proto()
			So(err, ShouldErrLike, `cannot parse "nerdz"`)
		})
	})
}

func TestCommitProto(t *testing.T) {
	t.Parallel()

	Convey(`Test Commit.Proto`, t, func() {
		c := &Commit{
			Commit: strings.Repeat("deadbeef", 5),
			Tree:   strings.Repeat("ac1df00d", 5),
			Parents: []string{
				strings.Repeat("d15c0bee", 5),
				strings.Repeat("daff0d11", 5),
			},
			Author:    User{"author", "author@example.com", "Wed Mar 09 03:46:18 2016"},
			Committer: User{"committer", "committer@example.com", "Wed Mar 09 03:46:18 2016"},
			Message:   "I am\na\nbanana",
		}

		Convey(`basic`, func() {
			cPB, err := c.Proto()
			So(err, ShouldBeNil)
			So(cPB, ShouldResemble, &git.Commit{
				Id:   bytes.Repeat([]byte{0xde, 0xad, 0xbe, 0xef}, 5),
				Tree: bytes.Repeat([]byte{0xac, 0x1d, 0xf0, 0x0d}, 5),
				Parents: [][]byte{
					bytes.Repeat([]byte{0xd1, 0x5c, 0x0b, 0xee}, 5),
					bytes.Repeat([]byte{0xda, 0xff, 0x0d, 0x11}, 5),
				},
				Author: &git.Commit_User{
					Name:  "author",
					Email: "author@example.com",
					Time:  &timestamp.Timestamp{Seconds: 1457495178},
				},
				Committer: &git.Commit_User{
					Name:  "committer",
					Email: "committer@example.com",
					Time:  &timestamp.Timestamp{Seconds: 1457495178},
				},
				Message: "I am\na\nbanana",
			})
		})

		Convey(`bad id`, func() {
			c.Commit = "nerp"
			_, err := c.Proto()
			So(err, ShouldErrLike, "decoding id")
		})

		Convey(`bad tree`, func() {
			c.Tree = "nerp"
			_, err := c.Proto()
			So(err, ShouldErrLike, "decoding tree")
		})

		Convey(`bad parent`, func() {
			c.Parents[0] = "nerp"
			_, err := c.Proto()
			So(err, ShouldErrLike, "decoding parent 0")
		})

		Convey(`bad author`, func() {
			c.Author.Time = "nerp"
			_, err := c.Proto()
			So(err, ShouldErrLike, "decoding author")
		})

		Convey(`bad committer`, func() {
			c.Committer.Time = "nerp"
			_, err := c.Proto()
			So(err, ShouldErrLike, "decoding committer")
		})

	})
}
