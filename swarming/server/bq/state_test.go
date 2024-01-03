// Copyright 2023 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package bq

import (
	"context"
	"testing"

	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/swarming/server/bq/taskspb"

	. "github.com/smartystreets/goconvey/convey"
)

func TestExportState(t *testing.T) {
	t.Parallel()
	Convey("With mocks", t, func() {
		setup := func() (context.Context, testclock.TestClock) {
			ctx, tc := testclock.UseTime(context.Background(), testclock.TestRecentTimeUTC)
			ctx = memory.Use(ctx)
			return ctx, tc
		}
		Convey("Can create export state key from CreateExportTask", func() {
			ctx, _ := setup()
			task := &taskspb.CreateExportTask{
				CloudProject: "foo",
				Dataset:      "bar",
				TableName:    "baz",
				Start:        timestamppb.New(testclock.TestRecentTimeUTC),
				Duration:     durationpb.New(100),
			}
			state := ExportState{
				Key:             exportStateKey(ctx, task),
				WriteStreamName: "stream",
			}
			err := datastore.Put(ctx, &state)
			So(err, ShouldBeNil)
		})
	})
}
