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

package bugupdater

import (
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/analysis/internal/tasks/taskspb"
	"go.chromium.org/luci/analysis/internal/testutil"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestSchedule(t *testing.T) {
	Convey(`TestSchedule`, t, func() {
		ctx, skdr := tq.TestingContext(testutil.TestingContext(), nil)

		task := &taskspb.UpdateBugs{
			Project:                   "chromium",
			ReclusteringAttemptMinute: timestamppb.New(time.Date(2025, time.January, 1, 12, 13, 0, 0, time.UTC)),
			Deadline:                  timestamppb.New(time.Date(2025, time.January, 1, 12, 28, 0, 0, time.UTC)),
		}
		expected := proto.Clone(task).(*taskspb.UpdateBugs)
		So(Schedule(ctx, task), ShouldBeNil)
		So(skdr.Tasks().Payloads()[0], ShouldResembleProto, expected)
	})
}

func TestValidate(t *testing.T) {
	Convey(`Validate`, t, func() {
		task := &taskspb.UpdateBugs{
			Project:                   "chromium",
			ReclusteringAttemptMinute: timestamppb.New(time.Date(2025, time.January, 1, 12, 13, 0, 0, time.UTC)),
			Deadline:                  timestamppb.New(time.Date(2025, time.January, 1, 12, 28, 0, 0, time.UTC)),
		}
		Convey(`baseline`, func() {
			err := validateTask(task)
			So(err, ShouldBeNil)
		})
		Convey(`invalid project`, func() {
			task.Project = ""
			err := validateTask(task)
			So(err, ShouldErrLike, "project: unspecified")
		})
		Convey(`invalid reclustering attempt minute`, func() {
			task.ReclusteringAttemptMinute = nil
			err := validateTask(task)
			So(err, ShouldErrLike, "reclustering_attempt_minute: missing or invalid timestamp")
		})
		Convey(`unaligned reclustering attempt minute`, func() {
			task.ReclusteringAttemptMinute = timestamppb.New(time.Date(2025, time.January, 1, 12, 13, 59, 0, time.UTC))
			err := validateTask(task)
			So(err, ShouldErrLike, "reclustering_attempt_minute: must be aligned to the start of a minute")
		})
		Convey(`invalid deadline`, func() {
			task.Deadline = nil
			err := validateTask(task)
			So(err, ShouldErrLike, "deadline: missing or invalid timestamp")
		})
	})
}
