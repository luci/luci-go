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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/analysis/internal/tasks/taskspb"
	"go.chromium.org/luci/analysis/internal/testutil"
)

func TestSchedule(t *testing.T) {
	ftt.Run(`TestSchedule`, t, func(t *ftt.Test) {
		ctx, skdr := tq.TestingContext(testutil.TestingContext(), nil)

		task := &taskspb.UpdateBugs{
			Project:                   "chromium",
			ReclusteringAttemptMinute: timestamppb.New(time.Date(2025, time.January, 1, 12, 13, 0, 0, time.UTC)),
			Deadline:                  timestamppb.New(time.Date(2025, time.January, 1, 12, 28, 0, 0, time.UTC)),
		}
		expected := proto.Clone(task).(*taskspb.UpdateBugs)
		assert.Loosely(t, Schedule(ctx, task), should.BeNil)
		assert.Loosely(t, skdr.Tasks().Payloads()[0], should.Match(expected))
	})
}

func TestValidate(t *testing.T) {
	ftt.Run(`Validate`, t, func(t *ftt.Test) {
		task := &taskspb.UpdateBugs{
			Project:                   "chromium",
			ReclusteringAttemptMinute: timestamppb.New(time.Date(2025, time.January, 1, 12, 13, 0, 0, time.UTC)),
			Deadline:                  timestamppb.New(time.Date(2025, time.January, 1, 12, 28, 0, 0, time.UTC)),
		}
		t.Run(`baseline`, func(t *ftt.Test) {
			err := validateTask(task)
			assert.Loosely(t, err, should.BeNil)
		})
		t.Run(`invalid project`, func(t *ftt.Test) {
			task.Project = ""
			err := validateTask(task)
			assert.Loosely(t, err, should.ErrLike("project: unspecified"))
		})
		t.Run(`invalid reclustering attempt minute`, func(t *ftt.Test) {
			task.ReclusteringAttemptMinute = nil
			err := validateTask(task)
			assert.Loosely(t, err, should.ErrLike("reclustering_attempt_minute: missing or invalid timestamp"))
		})
		t.Run(`unaligned reclustering attempt minute`, func(t *ftt.Test) {
			task.ReclusteringAttemptMinute = timestamppb.New(time.Date(2025, time.January, 1, 12, 13, 59, 0, time.UTC))
			err := validateTask(task)
			assert.Loosely(t, err, should.ErrLike("reclustering_attempt_minute: must be aligned to the start of a minute"))
		})
		t.Run(`invalid deadline`, func(t *ftt.Test) {
			task.Deadline = nil
			err := validateTask(task)
			assert.Loosely(t, err, should.ErrLike("deadline: missing or invalid timestamp"))
		})
	})
}
