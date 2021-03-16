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

package prjmanager

import (
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	"go.chromium.org/luci/auth/identity"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/config"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/gerrit"
	gf "go.chromium.org/luci/cv/internal/gerrit/gerritfake"
	"go.chromium.org/luci/cv/internal/gerrit/trigger"
	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/eventpb"
	"go.chromium.org/luci/cv/internal/run/runtest"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestComputeCLsDigest(t *testing.T) {
	t.Parallel()

	Convey("RunBuilder.computeCLsDigest works", t, func() {
		// This test mirrors the `test_attempt_key_hash` in CQDaemon's
		// pending_manager/test/base_test.py file.
		snapshotOf := func(host string, num int64) *changelist.Snapshot {
			return &changelist.Snapshot{
				Kind: &changelist.Snapshot_Gerrit{Gerrit: &changelist.Gerrit{
					Host: host,
					Info: &gerritpb.ChangeInfo{Number: num},
				}},
			}
		}
		epoch := time.Date(2020, time.December, 31, 0, 0, 0, 0, time.UTC)
		triggerAt := func(delay time.Duration) *run.Trigger {
			return &run.Trigger{Time: timestamppb.New(epoch.Add(delay))}
		}

		rb := RunBuilder{
			InputCLs: []RunBuilderCL{
				{
					Snapshot:    snapshotOf("b.example.com", 1),
					TriggerInfo: triggerAt(49999 * time.Microsecond),
				},
				{
					Snapshot:    snapshotOf("a.example.com", 2),
					TriggerInfo: triggerAt(777777 * time.Microsecond),
				},
			},
		}
		rb.computeCLsDigest()
		So(rb.runIDBuilder.version, ShouldEqual, 1)
		So(hex.EncodeToString(rb.runIDBuilder.digest), ShouldEqual,
			"28cb4b82698febb13483f5b2eb5e3ec19d5d77e9c503d5aefcb2b11a")
	})
}

func TestRunBuilder(t *testing.T) {
	t.Parallel()

	Convey("RunBuilder works", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		const lProject = "infra"
		const gHost = "x-review.example.com"
		const gProject = "infra/luci/luci-go"

		makeCI := func(n int) *gerritpb.ChangeInfo {
			votedAt := ct.Clock.Now()
			return gf.CI(n,
				gf.Project(gProject),
				gf.CQ(1, votedAt, gf.U("user-1")),
				gf.Updated(votedAt),
			)
		}
		makeSnapshot := func(ci *gerritpb.ChangeInfo) *changelist.Snapshot {
			min, cur, err := gerrit.EquivalentPatchsetRange(ci)
			So(err, ShouldBeNil)
			return &changelist.Snapshot{
				Kind: &changelist.Snapshot_Gerrit{Gerrit: &changelist.Gerrit{
					Host: gHost,
					Info: ci,
				}},
				LuciProject:           lProject,
				ExternalUpdateTime:    ci.GetUpdated(),
				Patchset:              int32(cur),
				MinEquivalentPatchset: int32(min),
			}
		}
		writeCL := func(snapshot *changelist.Snapshot) *changelist.CL {
			eid, err := changelist.GobID(snapshot.GetGerrit().GetHost(), snapshot.GetGerrit().GetInfo().GetNumber())
			So(err, ShouldBeNil)
			err = changelist.Update(ctx, eid, 0, changelist.UpdateFields{Snapshot: snapshot}, nil)
			So(err, ShouldBeNil)
			cl, err := eid.Get(ctx)
			So(err, ShouldBeNil)
			return cl
		}
		triggerOf := func(cl *changelist.CL) *run.Trigger {
			t := trigger.Find(cl.Snapshot.GetGerrit().GetInfo())
			So(t, ShouldNotBeNil)
			return t
		}

		ct.Clock.Add(time.Minute)
		cl1 := writeCL(makeSnapshot(makeCI(1)))
		ct.Clock.Add(time.Minute)
		cl2 := writeCL(makeSnapshot(makeCI(2)))
		cl2.IncompleteRuns = common.MakeRunIDs("expected/000-run")
		So(datastore.Put(ctx, cl2), ShouldBeNil)

		owner, err := identity.MakeIdentity("user:owner@example.com")
		So(err, ShouldBeNil)

		rb := RunBuilder{
			LUCIProject:              lProject,
			ConfigGroupID:            config.ConfigGroupID("sha256:cafe/cq-group"),
			OperationID:              "this-operation-id",
			Mode:                     run.DryRun,
			Owner:                    owner,
			ExpectedIncompleteRunIDs: common.MakeRunIDs("expected/000-run"),
			InputCLs: []RunBuilderCL{
				{
					ID:               cl1.ID,
					TriggerInfo:      triggerOf(cl1),
					ExpectedEVersion: 1,
					Snapshot:         cl1.Snapshot,
				},
				{
					ID:               cl2.ID,
					ExpectedEVersion: 1,
					TriggerInfo:      triggerOf(cl2),
					Snapshot:         cl2.Snapshot,
				},
			},
		}

		projectStateOffload := &ProjectStateOffload{
			Project:    datastore.MakeKey(ctx, ProjectKind, rb.LUCIProject),
			ConfigHash: "sha256:cafe",
			Status:     prjpb.Status_STARTED,
		}
		So(datastore.Put(ctx, projectStateOffload), ShouldBeNil)

		Convey("Checks preconditions", func() {
			Convey("No ProjectStateOffload", func() {
				So(datastore.Delete(ctx, projectStateOffload), ShouldBeNil)
				_, err := rb.Create(ctx)
				So(err, ShouldErrLike, "failed to load ProjectStateOffload")
				So(StateChangedTag.In(err), ShouldBeFalse)
				So(transient.Tag.In(err), ShouldBeFalse)
			})

			Convey("Mismatched project status", func() {
				projectStateOffload.Status = prjpb.Status_STOPPING
				So(datastore.Put(ctx, projectStateOffload), ShouldBeNil)
				_, err := rb.Create(ctx)
				So(err, ShouldErrLike, "status is STOPPING, expected STARTED")
				So(StateChangedTag.In(err), ShouldBeTrue)
				So(transient.Tag.In(err), ShouldBeFalse)
			})

			Convey("Mismatched project config", func() {
				projectStateOffload.ConfigHash = "wrong-hash"
				So(datastore.Put(ctx, projectStateOffload), ShouldBeNil)
				_, err := rb.Create(ctx)
				So(err, ShouldErrLike, "expected sha256:cafe")
				So(StateChangedTag.In(err), ShouldBeTrue)
				So(transient.Tag.In(err), ShouldBeFalse)
			})

			Convey("CL not exists", func() {
				So(datastore.Delete(ctx, cl2), ShouldBeNil)
				_, err := rb.Create(ctx)
				So(err, ShouldErrLike, fmt.Sprintf("CL %d doesn't exist", cl2.ID))
				So(StateChangedTag.In(err), ShouldBeFalse)
				So(transient.Tag.In(err), ShouldBeFalse)
			})

			Convey("Mismatched CL version", func() {
				rb.InputCLs[0].ExpectedEVersion = 11
				_, err := rb.Create(ctx)
				So(err, ShouldErrLike, fmt.Sprintf("CL %d changed since EVersion 11", cl1.ID))
				So(StateChangedTag.In(err), ShouldBeTrue)
				So(transient.Tag.In(err), ShouldBeFalse)
			})

			Convey("Unexpected IncompleteRun in a CL", func() {
				cl2.IncompleteRuns = common.MakeRunIDs("unexpected/111-run")
				So(datastore.Put(ctx, cl2), ShouldBeNil)
				_, err := rb.Create(ctx)
				So(err, ShouldErrLike, fmt.Sprintf(`CL %d has unexpected incomplete runs: [unexpected/111-run]`, cl2.ID))
				So(StateChangedTag.In(err), ShouldBeTrue)
				So(transient.Tag.In(err), ShouldBeFalse)
			})
		})

		const expectedRunID = "infra/9223372036854-1-6f432997511a722c89da1d8efb5d1a5d673e42430e87aa4426f57f86"

		Convey("First test to fail: check ID assumption", func() {
			// If this test fails due to change of runID scheme, update the constant
			// above.
			rb.computeCLsDigest()
			rb.computeRunID(ctx)
			So(rb.runID, ShouldEqual, expectedRunID)
		})

		Convey("Run already created", func() {

			Convey("by someone else", func() {
				err := datastore.Put(ctx, &run.Run{
					ID:                  expectedRunID,
					CreationOperationID: "concurrent runner",
				})
				So(err, ShouldBeNil)
				_, err = rb.Create(ctx)
				So(err, ShouldErrLike, `already created with OperationID "concurrent runner"`)
				So(StateChangedTag.In(err), ShouldBeFalse)
				So(transient.Tag.In(err), ShouldBeFalse)
			})

			Convey("by us", func() {
				err := datastore.Put(ctx, &run.Run{
					ID:                  expectedRunID,
					CreationOperationID: rb.OperationID,
				})
				So(err, ShouldBeNil)
				r, err := rb.Create(ctx)
				So(r, ShouldNotBeNil)
				So(err, ShouldBeNil)
			})
		})

		Convey("New Run is created", func() {
			ct.Clock.Add(time.Hour)
			r, err := rb.Create(ctx)
			So(err, ShouldBeNil)
			expectedRun := &run.Run{
				ID:         expectedRunID,
				EVersion:   1,
				CreateTime: datastore.RoundTime(ct.Clock.Now().UTC()),
				UpdateTime: datastore.RoundTime(ct.Clock.Now().UTC()),
				CLs:        common.CLIDs{cl1.ID, cl2.ID},
				Status:     run.Status_PENDING,

				CreationOperationID: rb.run.CreationOperationID,
				ConfigGroupID:       rb.ConfigGroupID,
				Mode:                rb.Mode,
				Owner:               rb.Owner,
			}
			So(r, ShouldResemble, expectedRun)
			for i, cl := range rb.cls {
				So(cl.EVersion, ShouldEqual, rb.InputCLs[i].ExpectedEVersion+1)
				So(cl.UpdateTime, ShouldResemble, r.CreateTime)
			}

			// Run is properly saved
			saved := &run.Run{ID: expectedRunID}
			So(datastore.Get(ctx, saved), ShouldBeNil)
			So(saved, ShouldResemble, expectedRun)

			for i := range rb.InputCLs {
				i := i
				Convey(fmt.Sprintf("RunCL %d-th is properly saved", i), func() {
					saved := &run.RunCL{
						ID:  rb.InputCLs[i].ID,
						Run: datastore.MakeKey(ctx, run.RunKind, expectedRunID),
					}
					So(datastore.Get(ctx, saved), ShouldBeNil)
					So(saved.Trigger, ShouldResembleProto, rb.InputCLs[i].TriggerInfo)
					So(saved.Detail, ShouldResembleProto, rb.cls[i].Snapshot)
				})
				Convey(fmt.Sprintf("CL %d-th is properly updated", i), func() {
					saved := &changelist.CL{ID: rb.InputCLs[i].ID}
					So(datastore.Get(ctx, saved), ShouldBeNil)
					So(saved.IncompleteRuns.ContainsSorted(expectedRunID), ShouldBeTrue)
					So(saved.UpdateTime, ShouldResemble, expectedRun.UpdateTime)
					So(saved.EVersion, ShouldEqual, rb.InputCLs[i].ExpectedEVersion+1)
				})
			}

			// TODO(tandrii): PM is notified.
			// This currently creates a package cycle.
			// pmtest.AssertInEventbox(ctx, lProject, &prjpb.Event{Event: &prjpb.Event_RunCreated{RunCreated: &prjpb.RunCreated{
			// 	RunId: string(r.ID),
			// }}})
			runtest.AssertInEventbox(ctx, r.ID, &eventpb.Event{Event: &eventpb.Event_Start{Start: &eventpb.Start{}}})
		})
	})
}
