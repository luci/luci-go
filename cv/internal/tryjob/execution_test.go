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

	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"

	. "github.com/smartystreets/goconvey/convey"
)

func TestLoadExecutionLogs(t *testing.T) {
	t.Parallel()

	Convey("LoadExecutionLogs works", t, func() {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		ev := int64(1)
		put := func(runID common.RunID, entries ...*ExecutionLogEntry) {
			So(datastore.Put(ctx, &executionLog{
				ID:  ev,
				Run: datastore.MakeKey(ctx, common.RunKind, string(runID)),
				Entries: &ExecutionLogEntries{
					Entries: entries,
				},
			}), ShouldBeNil)
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
		So(err, ShouldBeNil)
		So(ret, ShouldHaveLength, 4)
		So(ret[0].GetTryjobsLaunched().GetTryjobs()[0].GetId(), ShouldEqual, 1)
		So(ret[1].GetTryjobsLaunched().GetTryjobs()[0].GetId(), ShouldEqual, 2)
		So(ret[2].GetTryjobsEnded().GetTryjobs()[0].GetId(), ShouldEqual, 1)
		So(ret[3].GetTryjobsEnded().GetTryjobs()[0].GetId(), ShouldEqual, 2)
	})
}
