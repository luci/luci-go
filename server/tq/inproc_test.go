// Copyright 2020 The LUCI Authors.
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

package tq

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	taskspb "cloud.google.com/go/cloudtasks/apiv2/cloudtaskspb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"

	"go.chromium.org/luci/server/tq/internal/reminder"
	"go.chromium.org/luci/server/tq/internal/testutil"

	. "github.com/smartystreets/goconvey/convey"
)

func TestInProcSweeper(t *testing.T) {
	t.Parallel()

	Convey("Works", t, func() {
		var epoch = testclock.TestRecentTimeUTC
		const reminderKeySpaceBytes = 8
		const count = 200

		ctx, _ := testclock.UseTime(context.Background(), epoch)
		ctx = gologger.StdConfig.Use(ctx)
		ctx = logging.SetLevel(ctx, logging.Debug)

		db := &testutil.FakeDB{}
		ctx = db.Inject(ctx)

		sub := &submitter{}

		for i := 0; i < count; i++ {
			num := fmt.Sprintf("%d", i)
			hash := sha256.Sum256([]byte(num))
			r := &reminder.Reminder{
				ID:         hex.EncodeToString(hash[:reminderKeySpaceBytes]),
				FreshUntil: epoch.Add(-time.Minute),
			}
			r.AttachPayload(&reminder.Payload{
				CreateTaskRequest: &taskspb.CreateTaskRequest{
					Parent: num,
				},
			})
			So(db.SaveReminder(ctx, r), ShouldBeNil)
		}
		So(db.AllReminders(), ShouldHaveLength, count)

		sw := NewInProcSweeper(InProcSweeperOptions{
			SweepShards:             4,
			TasksPerScan:            15,
			SecondaryScanShards:     4,
			SubmitBatchSize:         8,
			SubmitConcurrentBatches: 3,
		})

		So(sw.sweep(ctx, sub, reminderKeySpaceBytes), ShouldBeNil)

		So(db.AllReminders(), ShouldHaveLength, 0)
		So(sub.reqs, ShouldHaveLength, count)
	})
}
