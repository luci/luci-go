// Copyright 2021 The LUCI Authors.
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

package run

import (
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"

	. "github.com/smartystreets/goconvey/convey"
)

func TestLoadRunLogEntries(t *testing.T) {
	t.Parallel()

	Convey("LoadRunLogEntries works", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		ev := int64(1)
		put := func(runID common.RunID, entries ...*LogEntry) {
			So(datastore.Put(ctx, &RunLog{
				Run:     datastore.MakeKey(ctx, RunKind, string(runID)),
				ID:      ev,
				Entries: &LogEntries{Entries: entries},
			}), ShouldBeNil)
			ev += 1
		}

		const run1 = common.RunID("rust/123-1-beef")
		const run2 = common.RunID("dart/789-2-cafe")

		put(
			run1,
			&LogEntry{
				Time: timestamppb.New(ct.Clock.Now()),
				Kind: &LogEntry_Created_{Created: &LogEntry_Created{
					ConfigGroupId: "fi/rst",
				}},
			},
		)
		ct.Clock.Add(time.Minute)
		put(
			run1,
			&LogEntry{
				Time: timestamppb.New(ct.Clock.Now()),
				Kind: &LogEntry_ConfigChanged_{ConfigChanged: &LogEntry_ConfigChanged{
					ConfigGroupId: "se/cond",
				}},
			},
			&LogEntry{
				Time: timestamppb.New(ct.Clock.Now()),
				Kind: &LogEntry_TryjobsRequirementUpdated_{TryjobsRequirementUpdated: &LogEntry_TryjobsRequirementUpdated{}},
			},
		)

		ct.Clock.Add(time.Minute)
		put(
			run2,
			&LogEntry{
				Time: timestamppb.New(ct.Clock.Now()),
				Kind: &LogEntry_Created_{Created: &LogEntry_Created{
					ConfigGroupId: "fi/rst-but-run2",
				}},
			},
		)

		out1, err := LoadRunLogEntries(ctx, run1)
		So(err, ShouldBeNil)
		So(out1, ShouldHaveLength, 3)
		So(out1[0].GetCreated().GetConfigGroupId(), ShouldResemble, "fi/rst")
		So(out1[1].GetConfigChanged().GetConfigGroupId(), ShouldResemble, "se/cond")
		So(out1[2].GetTryjobsRequirementUpdated(), ShouldNotBeNil)

		out2, err := LoadRunLogEntries(ctx, run2)
		So(err, ShouldBeNil)
		So(out2, ShouldHaveLength, 1)
		So(out2[0].GetCreated().GetConfigGroupId(), ShouldResemble, "fi/rst-but-run2")
	})
}
