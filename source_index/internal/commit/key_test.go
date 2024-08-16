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
	"testing"
	"time"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/source_index/internal/testutil"

	. "github.com/smartystreets/goconvey/convey"
)

func TestKey(t *testing.T) {
	Convey("Key", t, func() {
		ctx := testutil.SpannerTestContext(t)

		now := time.Date(2055, time.May, 5, 5, 5, 5, 5, time.UTC)
		ctx, _ = testclock.UseTime(ctx, now)

		key := Key{
			Host:       "chromium.googlesource.com",
			Repository: "chromium/src",
			CommitHash: "50791c81152633c73485745d8311fafde0e4935a",
		}
		commitToStore := Commit{
			Key: key,
			Position: &Position{
				Ref:    "refs/heads/main",
				Number: 10,
			},
		}

		Convey("ReadCommit", func() {
			Convey("with position", func() {
				So(SetForTesting(ctx, commitToStore), ShouldBeNil)

				readCommit, err := ReadCommit(span.Single(ctx), key)

				So(err, ShouldBeNil)
				So(readCommit, ShouldResemble, &commitToStore)
			})

			Convey("without position", func() {
				commitToStore.Position = nil
				So(SetForTesting(ctx, commitToStore), ShouldBeNil)

				readCommit, err := ReadCommit(span.Single(ctx), key)

				So(err, ShouldBeNil)
				So(readCommit, ShouldResemble, &commitToStore)
			})
		})

		Convey("Exits", func() {
			Convey("existing commit", func() {
				So(SetForTesting(ctx, commitToStore), ShouldBeNil)

				exists, err := Exists(span.Single(ctx), key)

				So(err, ShouldBeNil)
				So(exists, ShouldBeTrue)
			})

			Convey("non-existing commit", func() {
				commitToStore.CommitHash = "94f4b5c7c0bacc03caf215987a068db54b88af20"
				So(SetForTesting(ctx, commitToStore), ShouldBeNil)

				exists, err := Exists(span.Single(ctx), key)

				So(err, ShouldBeNil)
				So(exists, ShouldBeFalse)
			})
		})
	})
}
