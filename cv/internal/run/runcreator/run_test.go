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

package runcreator

import (
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/auth/identity"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/configs/prjcfg"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/gerrit"
	gf "go.chromium.org/luci/cv/internal/gerrit/gerritfake"
	"go.chromium.org/luci/cv/internal/gerrit/trigger"
	"go.chromium.org/luci/cv/internal/metrics"
	"go.chromium.org/luci/cv/internal/prjmanager"
	"go.chromium.org/luci/cv/internal/prjmanager/pmtest"
	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/eventpb"
	"go.chromium.org/luci/cv/internal/run/runtest"
	"go.chromium.org/luci/cv/internal/tryjob"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestComputeCLsDigest(t *testing.T) {
	t.Parallel()

	Convey("RunBuilder.computeCLsDigest works", t, func() {
		// This test mirrors the `test_attempt_key_hash` in CQDaemon's
		// pending_manager/test/gerrit_test.py file.
		snapshotOf := func(host string, num int64, rev string) *changelist.Snapshot {
			return &changelist.Snapshot{
				Kind: &changelist.Snapshot_Gerrit{Gerrit: &changelist.Gerrit{
					Host: host,
					Info: &gerritpb.ChangeInfo{Number: num, CurrentRevision: rev},
				}},
			}
		}
		epoch := time.Date(2020, time.December, 31, 0, 0, 0, 0, time.UTC)
		triggerAt := func(mode run.Mode, account int64, delay time.Duration) *run.Trigger {
			return &run.Trigger{
				Time:            timestamppb.New(epoch.Add(delay)),
				Mode:            string(mode),
				GerritAccountId: account,
			}
		}

		rb := Creator{
			InputCLs: []CL{
				{
					Snapshot:    snapshotOf("x-review.example.com", 1234567, "rev2"),
					TriggerInfo: triggerAt(run.FullRun, 006, 49999*time.Microsecond),
				},
				{
					Snapshot:    snapshotOf("y-review.example.com", 7654321, "rev3"),
					TriggerInfo: triggerAt(run.FullRun, 007, 777777*time.Microsecond),
				},
			},
		}
		rb.computeCLsDigest()
		So(rb.runIDBuilder.version, ShouldEqual, 1)
		So(hex.EncodeToString(rb.runIDBuilder.digest), ShouldEqual, "bc86ed248de55fb0")

		// The CLsDigest must be agnostic of input CLs order.
		rb2 := Creator{InputCLs: []CL{rb.InputCLs[1], rb.InputCLs[0]}}
		rb2.computeCLsDigest()
		So(hex.EncodeToString(rb2.runIDBuilder.digest), ShouldEqual, "bc86ed248de55fb0")
	})
}

func TestRunBuilder(t *testing.T) {
	t.Parallel()

	Convey("RunBuilder works", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp(t)
		defer cancel()
		pmNotifier := prjmanager.NewNotifier(ct.TQDispatcher)
		runNotifier := run.NewNotifier(ct.TQDispatcher)
		tjNotifier := tryjob.NewNotifier(ct.TQDispatcher)
		clMutator := changelist.NewMutator(ct.TQDispatcher, pmNotifier, runNotifier, tjNotifier)

		const lProject = "infra"
		const gHost = "x-review.example.com"
		const gProject = "infra/luci/luci-go"

		triggerer := gf.U("user-1")
		makeCI := func(n int) *gerritpb.ChangeInfo {
			votedAt := ct.Clock.Now()
			return gf.CI(n,
				gf.Project(gProject),
				gf.CQ(1, votedAt, triggerer),
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
			eid := changelist.MustGobID(snapshot.GetGerrit().GetHost(), snapshot.GetGerrit().GetInfo().GetNumber())
			cl := eid.MustCreateIfNotExists(ctx)
			cl.Snapshot = snapshot
			So(datastore.Put(ctx, cl), ShouldBeNil)
			return cl
		}
		cqVoteTriggerOf := func(cl *changelist.CL) *run.Trigger {
			trigger := trigger.Find(&trigger.FindInput{
				ChangeInfo:  cl.Snapshot.GetGerrit().GetInfo(),
				ConfigGroup: &cfgpb.ConfigGroup{},
			}).GetCqVoteTrigger()
			So(trigger, ShouldNotBeNil)
			return trigger
		}

		ct.Clock.Add(time.Minute)
		cl1 := writeCL(makeSnapshot(makeCI(1)))
		ct.Clock.Add(time.Minute)
		cl2 := writeCL(makeSnapshot(makeCI(2)))
		cl2.IncompleteRuns = common.MakeRunIDs("expected/000-run")
		So(datastore.Put(ctx, cl2), ShouldBeNil)

		owner, err := identity.MakeIdentity("user:owner@example.com")
		So(err, ShouldBeNil)

		rb := Creator{
			LUCIProject:              lProject,
			ConfigGroupID:            prjcfg.ConfigGroupID("sha256:cafe/cq-group"),
			OperationID:              "this-operation-id",
			Mode:                     run.DryRun,
			Owner:                    owner,
			CreatedBy:                identity.Identity(fmt.Sprintf("%s:%s", identity.User, triggerer.Email)),
			Options:                  &run.Options{},
			ExpectedIncompleteRunIDs: common.MakeRunIDs("expected/000-run"),
			InputCLs: []CL{
				{
					ID:               cl1.ID,
					TriggerInfo:      cqVoteTriggerOf(cl1),
					ExpectedEVersion: 1,
					Snapshot:         cl1.Snapshot,
				},
				{
					ID:               cl2.ID,
					ExpectedEVersion: 1,
					TriggerInfo:      cqVoteTriggerOf(cl2),
					Snapshot:         cl2.Snapshot,
				},
			},
			DepRuns: common.RunIDs{"dead-beef"},
		}

		projectStateOffload := &prjmanager.ProjectStateOffload{
			Project:    datastore.MakeKey(ctx, prjmanager.ProjectKind, rb.LUCIProject),
			ConfigHash: "sha256:cafe",
			Status:     prjpb.Status_STARTED,
			UpdateTime: ct.Clock.Now().UTC(),
		}
		So(datastore.Put(ctx, projectStateOffload), ShouldBeNil)

		Convey("Checks preconditions", func() {
			Convey("No ProjectStateOffload", func() {
				So(datastore.Delete(ctx, projectStateOffload), ShouldBeNil)
				_, err := rb.Create(ctx, clMutator, pmNotifier, runNotifier)
				So(err, ShouldErrLike, "failed to load ProjectStateOffload")
				So(StateChangedTag.In(err), ShouldBeFalse)
				So(transient.Tag.In(err), ShouldBeFalse)
			})

			Convey("Mismatched project status", func() {
				projectStateOffload.Status = prjpb.Status_STOPPING
				So(datastore.Put(ctx, projectStateOffload), ShouldBeNil)
				_, err := rb.Create(ctx, clMutator, pmNotifier, runNotifier)
				So(err, ShouldErrLike, "status is STOPPING, expected STARTED")
				So(StateChangedTag.In(err), ShouldBeTrue)
				So(transient.Tag.In(err), ShouldBeFalse)
			})

			Convey("Mismatched project config", func() {
				projectStateOffload.ConfigHash = "wrong-hash"
				So(datastore.Put(ctx, projectStateOffload), ShouldBeNil)
				_, err := rb.Create(ctx, clMutator, pmNotifier, runNotifier)
				So(err, ShouldErrLike, "expected sha256:cafe")
				So(StateChangedTag.In(err), ShouldBeTrue)
				So(transient.Tag.In(err), ShouldBeFalse)
			})

			Convey("Updated project config", func() {
				projectStateOffload.ConfigHash = "stale-hash"
				projectStateOffload.UpdateTime = ct.Clock.Now().UTC().Add(-1 * time.Hour)
				So(datastore.Put(ctx, projectStateOffload), ShouldBeNil)
				_, err := rb.Create(ctx, clMutator, pmNotifier, runNotifier)
				So(err, ShouldBeNil)
			})

			Convey("CL not exists", func() {
				So(datastore.Delete(ctx, cl2), ShouldBeNil)
				_, err := rb.Create(ctx, clMutator, pmNotifier, runNotifier)
				So(err, ShouldErrLike, fmt.Sprintf("CL %d doesn't exist", cl2.ID))
				So(StateChangedTag.In(err), ShouldBeFalse)
				So(transient.Tag.In(err), ShouldBeFalse)
			})

			Convey("Mismatched CL version", func() {
				rb.InputCLs[0].ExpectedEVersion = 11
				_, err := rb.Create(ctx, clMutator, pmNotifier, runNotifier)
				So(err, ShouldErrLike, fmt.Sprintf("CL %d changed since EVersion 11", cl1.ID))
				So(StateChangedTag.In(err), ShouldBeTrue)
				So(transient.Tag.In(err), ShouldBeFalse)
			})

			Convey("Unexpected IncompleteRun in a CL", func() {
				cl2.IncompleteRuns = common.MakeRunIDs("unexpected/111-run")
				So(datastore.Put(ctx, cl2), ShouldBeNil)
				_, err := rb.Create(ctx, clMutator, pmNotifier, runNotifier)
				So(err, ShouldErrLike, fmt.Sprintf(`CL %d has unexpected incomplete runs: [unexpected/111-run]`, cl2.ID))
				So(StateChangedTag.In(err), ShouldBeTrue)
				So(transient.Tag.In(err), ShouldBeFalse)
			})
		})

		const expectedRunID = "infra/9042331276854-1-afc7c13288093a6d"

		Convey("First test to fail: check ID assumption", func() {
			// If this test fails due to change of runID scheme, update the constant
			// above.
			rb.prepare(ct.Clock.Now())
			So(rb.runID, ShouldEqual, common.RunID(expectedRunID))
		})

		Convey("Run already created", func() {
			Convey("by someone else", func() {
				err := datastore.Put(ctx, &run.Run{
					ID:                  expectedRunID,
					CreationOperationID: "concurrent runner",
				})
				So(err, ShouldBeNil)
				_, err = rb.Create(ctx, clMutator, pmNotifier, runNotifier)
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
				r, err := rb.Create(ctx, clMutator, pmNotifier, runNotifier)
				So(err, ShouldBeNil)
				So(r, ShouldNotBeNil)
			})
		})

		Convey("ExpectedRunID works if CreateTime is given", func() {
			rb.CreateTime = ct.Clock.Now()
			// For realism and to prevent non-determinism in production,
			// make CreateTime in the past.
			ct.Clock.Add(time.Second)
			So(rb.ExpectedRunID(), ShouldResemble, common.RunID(expectedRunID))
		})

		Convey("ExpectedRunID panics if CreateTime is not given", func() {
			rb.CreateTime = time.Time{}
			So(func() { rb.ExpectedRunID() }, ShouldPanic)
		})

		Convey("New Run is created", func() {
			r, err := rb.Create(ctx, clMutator, pmNotifier, runNotifier)
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
				CreatedBy:           rb.CreatedBy,
				Options:             &run.Options{},
				DepRuns:             common.RunIDs{"dead-beef"},
			}
			So(r, runtest.ShouldResembleRun, expectedRun)

			for i, cl := range rb.cls {
				So(cl.EVersion, ShouldEqual, rb.InputCLs[i].ExpectedEVersion+1)
				So(cl.UpdateTime, ShouldResemble, r.CreateTime)
			}

			// Run is properly saved
			saved := &run.Run{ID: expectedRun.ID}
			So(datastore.Get(ctx, saved), ShouldBeNil)
			So(saved, runtest.ShouldResembleRun, expectedRun)

			for i := range rb.InputCLs {
				i := i
				Convey(fmt.Sprintf("RunCL %d-th is properly saved", i), func() {
					saved := &run.RunCL{
						ID:  rb.InputCLs[i].ID,
						Run: datastore.MakeKey(ctx, common.RunKind, expectedRunID),
					}
					So(datastore.Get(ctx, saved), ShouldBeNil)
					So(saved.ExternalID, ShouldEqual, rb.cls[i].ExternalID)
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

			// RunLog must contain the first entry for Creation.
			entries, err := run.LoadRunLogEntries(ctx, expectedRun.ID)
			So(err, ShouldBeNil)
			So(entries, ShouldHaveLength, 1)
			So(entries[0].GetCreated().GetConfigGroupId(), ShouldResemble, string(expectedRun.ConfigGroupID))

			// Created metric is sent.
			So(ct.TSMonSentValue(ctx, metrics.Public.RunCreated, lProject, "cq-group", string(run.DryRun)), ShouldEqual, 1)

			// Both PM and RM must be notified about new Run.
			pmtest.AssertInEventbox(ctx, lProject, &prjpb.Event{Event: &prjpb.Event_RunCreated{RunCreated: &prjpb.RunCreated{
				RunId: string(r.ID),
			}}})
			runtest.AssertInEventbox(ctx, r.ID, &eventpb.Event{Event: &eventpb.Event_Start{Start: &eventpb.Start{}}})
			// RM must have an immediate task to start working on a new Run.
			So(runtest.Runs(ct.TQ.Tasks()), ShouldResemble, common.RunIDs{r.ID})
		})
	})
}
