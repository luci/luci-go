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
	"sync"
	"testing"

	"go.chromium.org/luci/server/tq/internal/tqpb"
	"go.chromium.org/luci/server/tq/tqtesting"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestSweepRouting(t *testing.T) {
	t.Parallel()

	Convey("Works", t, func() {
		ctx := context.Background()

		disp := Dispatcher{}
		ctx, sched := TestingContext(ctx, &disp)

		mu := sync.Mutex{}
		calls := []*tqpb.SweepTask{}

		enqueue := sweepTaskRouting(&disp,
			DistributedSweeperOptions{TaskQueue: "zzz"},
			func(_ context.Context, task *tqpb.SweepTask) error {
				mu.Lock()
				calls = append(calls, task)
				mu.Unlock()
				return nil
			},
		)

		submitted := &tqpb.SweepTask{ShardCount: 123}
		enqueue(ctx, submitted)

		sched.Run(ctx, tqtesting.StopWhenDrained())
		So(calls, ShouldHaveLength, 1)
		So(calls[0], ShouldResembleProto, submitted)
	})
}
