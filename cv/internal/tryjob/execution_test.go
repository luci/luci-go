// Copyright 2022 The LUCI Authors.
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

package tryjob

import (
	"testing"
	"time"

	timestamppb "google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"
)

func TestLoadExecutionLogs(t *testing.T) {
	t.Parallel()

	ftt.Run("LoadExecutionLogs works", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		ev := int64(1)
		put := func(runID common.RunID, entries ...*ExecutionLogEntry) {
			assert.Loosely(t, datastore.Put(ctx, &executionLog{
				ID:  ev,
				Run: datastore.MakeKey(ctx, common.RunKind, string(runID)),
				Entries: &ExecutionLogEntries{
					Entries: entries,
				},
			}), should.BeNil)
			ev += 1
		}

		var runID = common.MakeRunID("test_proj", ct.Clock.Now().UTC(), 1, []byte("deadbeef"))

		put(
			runID,
			&ExecutionLogEntry{
				Time: timestamppb.New(ct.Clock.Now()),
				Kind: &ExecutionLogEntry_TryjobsLaunched_{
					TryjobsLaunched: &ExecutionLogEntry_TryjobsLaunched{
						Tryjobs: []*ExecutionLogEntry_TryjobSnapshot{
							{Id: 1},
						},
					},
				},
			},
			&ExecutionLogEntry{
				Time: timestamppb.New(ct.Clock.Now()),
				Kind: &ExecutionLogEntry_TryjobsLaunched_{
					TryjobsLaunched: &ExecutionLogEntry_TryjobsLaunched{
						Tryjobs: []*ExecutionLogEntry_TryjobSnapshot{
							{Id: 2},
						},
					},
				},
			},
		)
		ct.Clock.Add(time.Minute)
		put(
			runID,
			&ExecutionLogEntry{
				Time: timestamppb.New(ct.Clock.Now()),
				Kind: &ExecutionLogEntry_TryjobsEnded_{
					TryjobsEnded: &ExecutionLogEntry_TryjobsEnded{
						Tryjobs: []*ExecutionLogEntry_TryjobSnapshot{
							{Id: 1},
						},
					},
				},
			},
		)
		ct.Clock.Add(time.Minute)
		put(
			runID,
			&ExecutionLogEntry{
				Time: timestamppb.New(ct.Clock.Now()),
				Kind: &ExecutionLogEntry_TryjobsEnded_{
					TryjobsEnded: &ExecutionLogEntry_TryjobsEnded{
						Tryjobs: []*ExecutionLogEntry_TryjobSnapshot{
							{Id: 2},
						},
					},
				},
			},
		)

		ret, err := LoadExecutionLogs(ctx, runID)
		assert.NoErr(t, err)
		assert.Loosely(t, ret, should.HaveLength(4))
		assert.Loosely(t, ret[0].GetTryjobsLaunched().GetTryjobs()[0].GetId(), should.Equal(1))
		assert.Loosely(t, ret[1].GetTryjobsLaunched().GetTryjobs()[0].GetId(), should.Equal(2))
		assert.Loosely(t, ret[2].GetTryjobsEnded().GetTryjobs()[0].GetId(), should.Equal(1))
		assert.Loosely(t, ret[3].GetTryjobsEnded().GetTryjobs()[0].GetId(), should.Equal(2))
	})
}
