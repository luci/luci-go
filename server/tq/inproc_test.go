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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/server/tq/internal/reminder"
	"go.chromium.org/luci/server/tq/internal/testutil"
)

func TestInProcSweeper(t *testing.T) {
	t.Parallel()

	ftt.Run("Works", t, func(t *ftt.Test) {
		var epoch = testclock.TestRecentTimeUTC
		const reminderKeySpaceBytes = 8
		const count = 200

		ctx, _ := testclock.UseTime(context.Background(), epoch)
		ctx = gologger.StdConfig.Use(ctx)
		ctx = logging.SetLevel(ctx, logging.Debug)

		db := &testutil.FakeDB{}
		ctx = db.Inject(ctx)

		sub := &submitter{}

		for i := range count {
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
			assert.Loosely(t, db.SaveReminder(ctx, r), should.BeNil)
		}
		assert.Loosely(t, db.AllReminders(), should.HaveLength(count))

		sw := NewInProcSweeper(InProcSweeperOptions{
			SweepShards:             4,
			TasksPerScan:            15,
			SecondaryScanShards:     4,
			SubmitBatchSize:         8,
			SubmitConcurrentBatches: 3,
		})

		assert.Loosely(t, sw.sweep(ctx, sub, reminderKeySpaceBytes), should.BeNil)

		assert.Loosely(t, db.AllReminders(), should.HaveLength(0))
		assert.Loosely(t, sub.reqs, should.HaveLength(count))
	})
}
