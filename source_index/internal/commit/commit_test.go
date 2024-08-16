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
	"context"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/source_index/internal/testutil"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCommit(t *testing.T) {
	Convey("Commit", t, func() {
		ctx := testutil.SpannerTestContext(t)

		now := time.Date(2055, time.May, 5, 5, 5, 5, 5, time.UTC)
		ctx, _ = testclock.UseTime(ctx, now)

		Convey("SaveUnverified", func() {
			commitToSave := Commit{
				Key: Key{
					Host:       "chromium.googlesource.com",
					Repository: "chromium/src",
					CommitHash: "50791c81152633c73485745d8311fafde0e4935a",
				},
				Position: &Position{
					Ref:    "refs/heads/main",
					Number: 10,
				},
			}

			saveCommit := func() error {
				_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
					span.BufferWrite(ctx, commitToSave.SaveUnverified())
					return nil
				})
				return err
			}

			assertCommitExpected := func() {
				readCommits, err := ReadAllForTesting(span.Single(ctx))
				So(err, ShouldBeNil)
				So(readCommits, ShouldHaveLength, 1)
				So(readCommits[0], ShouldResemble, commitToSave)
			}

			Convey("with position", func() {
				err := saveCommit()

				So(err, ShouldBeNil)
				assertCommitExpected()
			})

			Convey("without position", func() {
				commitToSave.Position = nil

				err := saveCommit()

				So(err, ShouldBeNil)
				assertCommitExpected()
			})
		})
	})
}
