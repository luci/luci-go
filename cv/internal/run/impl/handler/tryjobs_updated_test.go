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

package handler

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"go.chromium.org/luci/auth/identity"
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	cvbqpb "go.chromium.org/luci/cv/api/bigquery/v1"
	migrationpb "go.chromium.org/luci/cv/api/migration"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/migration"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/impl/state"
	"go.chromium.org/luci/cv/internal/tryjob"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestOnCQDTryjobsUpdated(t *testing.T) {
	t.Parallel()

	Convey("OnCQDTryjobsUpdated", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		const (
			lProject = "infra"
			bbHost   = "bb.example.com"
		)
		bbTryjobDef := func(projSlashBucketSlashBuilder string) *tryjob.Definition {
			parts := strings.Split(projSlashBucketSlashBuilder, "/")
			if len(parts) != 3 {
				panic(fmt.Errorf(`invalid bbTryjobDef(%q): must be like "project/bucket/builder"`, projSlashBucketSlashBuilder))
			}
			return &tryjob.Definition{
				Backend: &tryjob.Definition_Buildbucket_{
					Buildbucket: &tryjob.Definition_Buildbucket{
						Host: bbHost,
						Builder: &buildbucketpb.BuilderID{
							Project: parts[0],
							Bucket:  parts[1],
							Builder: parts[2],
						},
					},
				},
			}
		}

		rid := common.MakeRunID(lProject, ct.Clock.Now(), 1, []byte("deadbeef"))
		rs := &state.RunState{
			Run: run.Run{
				ID:         rid,
				Status:     run.Status_RUNNING,
				CreateTime: ct.Clock.Now().UTC().Add(-2 * time.Minute),
				StartTime:  ct.Clock.Now().UTC().Add(-1 * time.Minute),
				CLs:        common.CLIDs{1},
			},
		}
		h, deps := makeTestHandler(&ct)

		mig := migration.MigrationServer{RunNotifier: deps.rm}
		report := func(ts ...*migrationpb.Tryjob) {
			ctx := auth.WithState(ctx, &authtest.FakeState{
				Identity:             identity.Identity("project:infra"),
				PeerIdentityOverride: "user:cqdaemon@example.com",
			})
			_, err := mig.ReportTryjobs(ctx, &migrationpb.ReportTryjobsRequest{
				ConfigRevision: "doesn't matter",
				RunId:          string(rid),
				Tryjobs:        ts,
			})
			So(err, ShouldBeNil)
		}

		tryjobCreateTime := timestamppb.New(ct.Clock.Now().Add(-time.Hour))
		makeTryjob := func(id int64, s migrationpb.TryjobStatus, o cvbqpb.Build_Origin, criticality bool) *migrationpb.Tryjob {
			t := &migrationpb.Tryjob{
				Builder:    bbTryjobDef(fmt.Sprintf("%s/try/bldr-%d", lProject, id)).GetBuildbucket().GetBuilder(),
				CreateTime: tryjobCreateTime,
				Status:     s,
				Build: &cvbqpb.Build{
					Host:     bbHost,
					Id:       id,
					Origin:   o,
					Critical: criticality,
				},
			}
			return t
		}

		Convey("Update state when Run is RUNNING", func() {
			// Send the first report.
			ct.Clock.Add(time.Minute)
			reported1001 := makeTryjob(1001, migrationpb.TryjobStatus_RUNNING, cvbqpb.Build_REUSED, true)
			report(reported1001)
			expected1001 := &run.Tryjob{
				Definition: bbTryjobDef(fmt.Sprintf("%s/try/bldr-1001", lProject)),
				CqdDerived: true,
				Id:         0,
				Eversion:   0,
				ExternalId: fmt.Sprintf("buildbucket/%s/1001", bbHost),
				Reused:     true,
				Critical:   true,
				Status:     tryjob.Status_TRIGGERED,
				Result: &tryjob.Result{
					CreateTime: tryjobCreateTime,
					Backend: &tryjob.Result_Buildbucket_{Buildbucket: &tryjob.Result_Buildbucket{
						Id:     1001,
						Status: buildbucketpb.Status_STARTED,
					}},
				},
			}
			// Send the second report.
			ct.Clock.Add(time.Minute)
			reported1002 := makeTryjob(1002, migrationpb.TryjobStatus_PENDING, cvbqpb.Build_NOT_REUSED, false)
			report(reported1001, reported1002)
			expected1002 := &run.Tryjob{
				Definition: bbTryjobDef(fmt.Sprintf("%s/try/bldr-1002", lProject)),
				CqdDerived: true,
				Id:         0,
				Eversion:   0,
				ExternalId: fmt.Sprintf("buildbucket/%s/1002", bbHost),
				Reused:     false,
				Critical:   false,
				Status:     tryjob.Status_TRIGGERED,
				Result: &tryjob.Result{
					CreateTime: tryjobCreateTime,
					Backend: &tryjob.Result_Buildbucket_{Buildbucket: &tryjob.Result_Buildbucket{
						Id:     1002,
						Status: buildbucketpb.Status_SCHEDULED,
					}},
				},
			}

			// First import processes all available reports.
			res, err := h.OnCQDTryjobsUpdated(ctx, rs)
			So(err, ShouldBeNil)
			So(res.SideEffectFn, ShouldBeNil)
			So(res.PreserveEvents, ShouldBeFalse)
			So(res.State.Tryjobs.GetTryjobs(), ShouldHaveLength, 2)
			// For easier to debug test, compare individual tryjobs first.
			So(res.State.Tryjobs.GetTryjobs()[0], ShouldResembleProto, expected1001)
			So(res.State.Tryjobs.GetTryjobs()[1], ShouldResembleProto, expected1002)
			// And now compare entire Tryjobs state.
			So(res.State.Tryjobs, ShouldResembleProto, &run.Tryjobs{
				CqdUpdateTime: timestamppb.New(ct.Clock.Now()),
				Tryjobs:       []*run.Tryjob{expected1001, expected1002},
				Requirement:   nil, // not important in this test
			})
			// Finally, there must be 2 diffs reported to RunLog.
			So(res.State.LogEntries, ShouldHaveLength, 2)

			// Follow up imports only not yet read reports.
			priorState := res.State
			backup := proto.Clone(priorState.Tryjobs).(*run.Tryjobs)
			ct.Clock.Add(time.Minute)
			reported1001.Status = migrationpb.TryjobStatus_FAILED
			reported1002.Status = migrationpb.TryjobStatus_SUCCEEDED
			report(reported1001, reported1002)
			res, err = h.OnCQDTryjobsUpdated(ctx, &state.RunState{Run: priorState.Run})
			So(err, ShouldBeNil)
			// Prior state must not be mutated by accident.
			So(priorState.Tryjobs, ShouldResembleProto, backup)
			// And there should be exactly 1 log entry.
			So(res.State.LogEntries, ShouldHaveLength, 1)
			// There must still be 2 tryjobs, but with diff statuses now.
			So(res.State.Tryjobs.GetTryjobs(), ShouldHaveLength, 2)
			So(res.State.Tryjobs.GetTryjobs()[0].GetStatus(), ShouldEqual, tryjob.Status_ENDED)
			So(res.State.Tryjobs.GetTryjobs()[0].GetResult().GetStatus(), ShouldEqual, tryjob.Result_FAILED_PERMANENTLY)
			So(res.State.Tryjobs.GetTryjobs()[1].GetStatus(), ShouldEqual, tryjob.Status_ENDED)
			So(res.State.Tryjobs.GetTryjobs()[1].GetResult().GetStatus(), ShouldEqual, tryjob.Result_SUCCEEDED)

			// Follow up report with the same tryjobs doesn't result in new Run Log
			// entry.
			ct.Clock.Add(time.Minute)
			report(reported1001, reported1002)
			res, err = h.OnCQDTryjobsUpdated(ctx, &state.RunState{Run: res.State.Run})
			So(err, ShouldBeNil)
			So(res.State.LogEntries, ShouldBeEmpty)

			// Follow up report may drop tryjobs.
			// This can happen iff a tryjob was reused from prior Run, but ultimately
			// failed, at which point CQDaemon would stop tracking it.
			ct.Clock.Add(time.Minute)
			report(reported1002)
			res, err = h.OnCQDTryjobsUpdated(ctx, &state.RunState{Run: res.State.Run})
			So(err, ShouldBeNil)
			So(res.State.Tryjobs.GetTryjobs(), ShouldHaveLength, 1)
			So(res.State.Tryjobs.GetTryjobs()[0].GetExternalId(), ShouldResemble, expected1002.GetExternalId())
			So(res.State.LogEntries, ShouldBeEmpty)
		})

		statuses := []run.Status{
			run.Status_SUCCEEDED,
			run.Status_FAILED,
			run.Status_CANCELLED,
			run.Status_WAITING_FOR_SUBMISSION,
			run.Status_SUBMITTING,
		}
		for _, status := range statuses {
			Convey(fmt.Sprintf("Noop when Run is %s", status), func() {
				rs.Status = status
				res, err := h.OnCQDTryjobsUpdated(ctx, rs)
				So(err, ShouldBeNil)
				So(res.State, ShouldEqual, rs)
				So(res.SideEffectFn, ShouldBeNil)
				switch {
				case run.IsEnded(status):
					So(res.PreserveEvents, ShouldBeFalse)
				default:
					So(res.PreserveEvents, ShouldBeTrue)
				}
			})
		}
	})
}
