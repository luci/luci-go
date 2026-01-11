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

package join

import (
	"context"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/common/tsmon"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/tq"

	controlpb "go.chromium.org/luci/analysis/internal/ingestion/control/proto"
	"go.chromium.org/luci/analysis/internal/tasks/taskspb"
)

func TestJoinRootInvocation(t *testing.T) {
	ftt.Run("JoinRootInvocation", t, func(t *ftt.Test) {
		ctx, _ := tsmon.WithDummyInMemory(context.Background())
		ctx, taskScheduler := tq.TestingContext(ctx, nil)

		createTime := timestamppb.New(time.Date(2025, time.January, 1, 12, 0, 0, 0, time.UTC))
		notification := &rdbpb.RootInvocationFinalizedNotification{
			RootInvocation: &rdbpb.RootInvocation{
				Name:             "rootInvocations/inv-123",
				Realm:            "project:realm",
				RootInvocationId: "inv-123",
				CreateTime:       createTime,
			},
			ResultdbHost: "rdb-host",
		}

		t.Run("schedules ingestion task", func(t *ftt.Test) {
			processed, err := JoinRootInvocation(ctx, notification)
			assert.NoErr(t, err)
			assert.Loosely(t, processed, should.BeTrue)

			assert.Loosely(t, taskScheduler.Tasks().Payloads(), should.HaveLength(1))
			task := taskScheduler.Tasks().Payloads()[0].(*taskspb.IngestTestVerdicts)
			assert.Loosely(t, task, should.Match(&taskspb.IngestTestVerdicts{
				IngestionId:   "rootInvocation/rdb-host/inv-123",
				PartitionTime: createTime,
				Project:       "project",
				RootInvocation: &controlpb.RootInvocationResult{
					ResultdbHost:     "rdb-host",
					RootInvocationId: "inv-123",
					CreationTime:     createTime,
				},
				TaskIndex: 1,
			}))
		})
	})
}
