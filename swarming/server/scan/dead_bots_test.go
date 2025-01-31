// Copyright 2025 The LUCI Authors.
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

package scan

import (
	"context"
	"sort"
	"sync"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"

	"go.chromium.org/luci/swarming/server/model"
)

func TestDeadBotDetector(t *testing.T) {
	t.Parallel()

	testTime := testclock.TestRecentTimeUTC.Round(time.Second)

	ctx := memory.Use(context.Background())
	ctx, _ = testclock.UseTime(ctx, testTime)

	var l sync.Mutex
	var attempted []string

	detector := &DeadBotDetector{
		BotDeathTimeout: 10 * time.Second,

		submitUpdate: func(ctx context.Context, update *model.BotInfoUpdate) (*model.SubmittedBotInfoUpdate, error) {
			l.Lock()
			attempted = append(attempted, update.BotID)
			l.Unlock()
			if update.BotID == "will-fail-to-become-dead" {
				return nil, errors.Reason("boo").Err()
			}
			return &model.SubmittedBotInfoUpdate{}, nil
		},
	}

	err := RunBotVisitor(ctx, detector, []FakeBot{
		{
			ID:       "alive",
			Dead:     false,
			LastSeen: testTime.Add(-9 * time.Second),
		},
		{
			ID:       "will-become-dead",
			Dead:     false,
			LastSeen: testTime.Add(-11 * time.Second),
		},
		{
			ID:       "already-dead",
			Dead:     true,
			LastSeen: testTime.Add(-11 * time.Second),
		},
		{
			ID:       "will-fail-to-become-dead",
			Dead:     false,
			LastSeen: testTime.Add(-11 * time.Second),
		},
		{
			ID:         "no-last-seen",
			NoLastSeen: true,
		},
	})
	assert.That(t, err, should.ErrLike("failed to mark"))

	sort.Strings(attempted)
	assert.That(t, attempted, should.Match([]string{
		"will-become-dead",
		"will-fail-to-become-dead",
	}))
}
