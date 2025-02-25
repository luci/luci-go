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
	"encoding/json"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/proto/git"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestTimestamp(t *testing.T) {
	t.Parallel()

	ftt.Run("Marshal and Unmarshal ts", t, func(t *ftt.Test) {
		// Nanoseconds must be zero because the string format in between
		// does not contain nanoseconds.
		tBefore := ts{time.Date(12, 2, 5, 6, 1, 3, 0, time.UTC)}
		bytes, err := json.Marshal(tBefore)
		assert.Loosely(t, err, should.BeNil)

		var tAfter ts
		err = json.Unmarshal(bytes, &tAfter)
		assert.Loosely(t, err, should.BeNil)

		assert.Loosely(t, tBefore, should.Match(tAfter))
	})
}

func TestUser(t *testing.T) {
	t.Parallel()

	ftt.Run(`Test user.Proto`, t, func(t *ftt.Test) {
		u := &user{
			Name:  "Some name",
			Email: "some.name@example.com",
			Time:  ts{time.Date(2016, 3, 9, 3, 46, 18, 0, time.UTC)},
		}

		t.Run(`basic`, func(t *ftt.Test) {
			uPB, err := u.Proto()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, uPB, should.Match(&git.Commit_User{
				Name:  "Some name",
				Email: "some.name@example.com",
				Time: &timestamppb.Timestamp{
					Seconds: 1457495178,
				},
			}))
		})

		t.Run(`empty ts`, func(t *ftt.Test) {
			u.Time = ts{}
			uPB, err := u.Proto()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, uPB, should.Match(&git.Commit_User{
				Name:  "Some name",
				Email: "some.name@example.com",
			}))
		})
	})
}

func TestCommit(t *testing.T) {
	t.Parallel()

	ftt.Run(`Test commit.Proto`, t, func(t *ftt.Test) {
		c := &commit{
			Commit: strings.Repeat("deadbeef", 5),
			Tree:   strings.Repeat("ac1df00d", 5),
			Parents: []string{
				strings.Repeat("d15c0bee", 5),
				strings.Repeat("daff0d11", 5),
			},
			Author:    user{"author", "author@example.com", ts{time.Date(2016, 3, 9, 3, 46, 18, 0, time.UTC)}},
			Committer: user{"committer", "committer@example.com", ts{time.Date(2016, 3, 9, 3, 46, 18, 0, time.UTC)}},
			Message:   "I am\na\nbanana",
		}

		t.Run(`basic`, func(t *ftt.Test) {
			cPB, err := c.Proto()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, cPB, should.Match(&git.Commit{
				Id:   strings.Repeat("deadbeef", 5),
				Tree: strings.Repeat("ac1df00d", 5),
				Parents: []string{
					strings.Repeat("d15c0bee", 5),
					strings.Repeat("daff0d11", 5),
				},
				Author: &git.Commit_User{
					Name:  "author",
					Email: "author@example.com",
					Time:  &timestamppb.Timestamp{Seconds: 1457495178},
				},
				Committer: &git.Commit_User{
					Name:  "committer",
					Email: "committer@example.com",
					Time:  &timestamppb.Timestamp{Seconds: 1457495178},
				},
				Message: "I am\na\nbanana",
			}))
		})
	})
}
