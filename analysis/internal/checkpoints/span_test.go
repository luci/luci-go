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

package checkpoints

import (
	"context"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/analysis/internal/testutil"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCheckpoints(t *testing.T) {
	Convey("With Spanner context", t, func() {
		ctx := testutil.SpannerTestContext(t)

		now := time.Date(2055, time.May, 5, 5, 5, 5, 5, time.UTC)
		ctx, _ = testclock.UseTime(ctx, now)

		Convey("Exists", func() {
			Convey("Does not exist", func() {
				key := Key{
					Project:    "project",
					ResourceID: "resource-id",
					ProcessID:  "process-id",
					Uniquifier: "uniqifier",
				}
				exists, err := Exists(span.Single(ctx), key)
				So(err, ShouldBeNil)
				So(exists, ShouldBeFalse)
			})
			Convey("Exists", func() {
				key := Key{
					Project:    "project",
					ResourceID: "resource-id",
					ProcessID:  "process-id",
					Uniquifier: "uniqifier",
				}
				_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
					ms := Insert(ctx, key, time.Hour)
					span.BufferWrite(ctx, ms)
					return nil
				})
				So(err, ShouldBeNil)

				exists, err := Exists(span.Single(ctx), key)
				So(err, ShouldBeNil)
				So(exists, ShouldBeTrue)
			})
		})
		Convey("Insert", func() {
			key := Key{
				Project:    "project",
				ResourceID: "resource-id",
				ProcessID:  "process-id",
				Uniquifier: "uniqifier",
			}
			_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
				ms := Insert(ctx, key, time.Hour)
				span.BufferWrite(ctx, ms)
				return nil
			})
			So(err, ShouldBeNil)

			checkpoints, err := ReadAllForTesting(span.Single(ctx))
			So(err, ShouldBeNil)
			So(checkpoints, ShouldHaveLength, 1)
			So(checkpoints[0].Key, ShouldResemble, key)
			So(checkpoints[0].CreationTime, ShouldNotBeZeroValue)
			So(checkpoints[0].ExpiryTime, ShouldEqual, now.Add(time.Hour))
		})
	})
}
