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

	"github.com/google/go-cmp/cmp/cmpopts"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/auth/identity"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/registry"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
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
)

func init() {
	registry.RegisterCmpOption(cmpopts.IgnoreUnexported(run.Run{}))
}

func TestComputeCLsDigest(t *testing.T) {
	t.Parallel()

	ftt.Run("RunBuilder.computeCLsDigest works", t, func(t *ftt.Test) {
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
		assert.Loosely(t, rb.runIDBuilder.version, should.Equal(1))
		assert.Loosely(t, hex.EncodeToString(rb.runIDBuilder.digest), should.Equal("bc86ed248de55fb0"))

		// The CLsDigest must be agnostic of input CLs order.
		rb2 := Creator{InputCLs: []CL{rb.InputCLs[1], rb.InputCLs[0]}}
		rb2.computeCLsDigest()
		assert.Loosely(t, hex.EncodeToString(rb2.runIDBuilder.digest), should.Equal("bc86ed248de55fb0"))
	})
}

func TestRunBuilder(t *testing.T) {
	t.Parallel()

	ftt.Run("RunBuilder works", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)
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
			assert.NoErr(t, err)
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
			assert.NoErr(t, datastore.Put(ctx, cl))
			return cl
		}
		cqVoteTriggerOf := func(cl *changelist.CL) *run.Trigger {
			trigger := trigger.Find(&trigger.FindInput{
				ChangeInfo:  cl.Snapshot.GetGerrit().GetInfo(),
				ConfigGroup: &cfgpb.ConfigGroup{},
			}).GetCqVoteTrigger()
			assert.Loosely(t, trigger, should.NotBeNil)
			return trigger
		}

		ct.Clock.Add(time.Minute)
		cl1 := writeCL(makeSnapshot(makeCI(1)))
		ct.Clock.Add(time.Minute)
		cl2 := writeCL(makeSnapshot(makeCI(2)))
		cl2.IncompleteRuns = common.MakeRunIDs("expected/000-run")
		assert.NoErr(t, datastore.Put(ctx, cl2))

		owner, err := identity.MakeIdentity("user:owner@example.com")
		assert.NoErr(t, err)
		trIdentity := identity.Identity(fmt.Sprintf("%s:%s", identity.User, triggerer.Email))
		cls := []CL{
			{
				ID:               cl1.ID,
				TriggerInfo:      cqVoteTriggerOf(cl1),
				ExpectedEVersion: 1,
				Snapshot:         cl1.Snapshot,
			},
			{
				ID:               cl2.ID,
				ExpectedEVersion: 1,
				Snapshot:         cl2.Snapshot,
			},
		}
		rb := Creator{
			LUCIProject:              lProject,
			ConfigGroupID:            prjcfg.ConfigGroupID("sha256:cafe/cq-group"),
			OperationID:              "this-operation-id",
			Mode:                     run.DryRun,
			Owner:                    owner,
			CreatedBy:                trIdentity,
			BilledTo:                 trIdentity,
			Options:                  &run.Options{},
			ExpectedIncompleteRunIDs: common.MakeRunIDs("expected/000-run"),
			InputCLs:                 cls,
			RootCL:                   cls[0],
			DepRuns:                  common.RunIDs{"dead-beef"},
		}

		projectStateOffload := &prjmanager.ProjectStateOffload{
			Project:    datastore.MakeKey(ctx, prjmanager.ProjectKind, rb.LUCIProject),
			ConfigHash: "sha256:cafe",
			Status:     prjpb.Status_STARTED,
			UpdateTime: ct.Clock.Now().UTC(),
		}
		assert.NoErr(t, datastore.Put(ctx, projectStateOffload))

		t.Run("Checks preconditions", func(t *ftt.Test) {
			t.Run("No ProjectStateOffload", func(t *ftt.Test) {
				assert.NoErr(t, datastore.Delete(ctx, projectStateOffload))
				_, err := rb.Create(ctx, clMutator, pmNotifier, runNotifier)
				assert.ErrIsLike(t, err, "failed to load ProjectStateOffload")
				assert.Loosely(t, StateChangedTag.In(err), should.BeFalse)
				assert.Loosely(t, transient.Tag.In(err), should.BeFalse)
			})

			t.Run("Mismatched project status", func(t *ftt.Test) {
				projectStateOffload.Status = prjpb.Status_STOPPING
				assert.NoErr(t, datastore.Put(ctx, projectStateOffload))
				_, err := rb.Create(ctx, clMutator, pmNotifier, runNotifier)
				assert.ErrIsLike(t, err, "status is STOPPING, expected STARTED")
				assert.Loosely(t, StateChangedTag.In(err), should.BeTrue)
				assert.Loosely(t, transient.Tag.In(err), should.BeFalse)
			})

			t.Run("Mismatched project config", func(t *ftt.Test) {
				projectStateOffload.ConfigHash = "wrong-hash"
				assert.NoErr(t, datastore.Put(ctx, projectStateOffload))
				_, err := rb.Create(ctx, clMutator, pmNotifier, runNotifier)
				assert.ErrIsLike(t, err, "expected sha256:cafe")
				assert.Loosely(t, StateChangedTag.In(err), should.BeTrue)
				assert.Loosely(t, transient.Tag.In(err), should.BeFalse)
			})

			t.Run("Updated project config", func(t *ftt.Test) {
				projectStateOffload.ConfigHash = "stale-hash"
				projectStateOffload.UpdateTime = ct.Clock.Now().UTC().Add(-1 * time.Hour)
				assert.NoErr(t, datastore.Put(ctx, projectStateOffload))
				_, err := rb.Create(ctx, clMutator, pmNotifier, runNotifier)
				assert.NoErr(t, err)
			})

			t.Run("CL not exists", func(t *ftt.Test) {
				assert.NoErr(t, datastore.Delete(ctx, cl2))
				_, err := rb.Create(ctx, clMutator, pmNotifier, runNotifier)
				assert.ErrIsLike(t, err, fmt.Sprintf("CL %d doesn't exist", cl2.ID))
				assert.Loosely(t, StateChangedTag.In(err), should.BeFalse)
				assert.Loosely(t, transient.Tag.In(err), should.BeFalse)
			})

			t.Run("Mismatched CL version", func(t *ftt.Test) {
				rb.InputCLs[0].ExpectedEVersion = 11
				_, err := rb.Create(ctx, clMutator, pmNotifier, runNotifier)
				assert.ErrIsLike(t, err, fmt.Sprintf("CL %d changed since EVersion 11", cl1.ID))
				assert.Loosely(t, StateChangedTag.In(err), should.BeTrue)
				assert.Loosely(t, transient.Tag.In(err), should.BeFalse)
			})

			t.Run("Unexpected IncompleteRun in a CL", func(t *ftt.Test) {
				cl2.IncompleteRuns = common.MakeRunIDs("unexpected/111-run")
				assert.NoErr(t, datastore.Put(ctx, cl2))
				_, err := rb.Create(ctx, clMutator, pmNotifier, runNotifier)
				assert.ErrIsLike(t, err, fmt.Sprintf(`CL %d has unexpected incomplete runs: [unexpected/111-run]`, cl2.ID))
				assert.Loosely(t, StateChangedTag.In(err), should.BeTrue)
				assert.Loosely(t, transient.Tag.In(err), should.BeFalse)
			})
		})

		const expectedRunID = "infra/9042331276854-1-13661a806e98be7e"

		t.Run("First test to fail: check ID assumption", func(t *ftt.Test) {
			// If this test fails due to change of runID scheme, update the constant
			// above.
			rb.prepare(ct.Clock.Now())
			assert.Loosely(t, rb.runID, should.Equal(common.RunID(expectedRunID)))
		})

		t.Run("Run already created", func(t *ftt.Test) {
			t.Run("by someone else", func(t *ftt.Test) {
				err := datastore.Put(ctx, &run.Run{
					ID:                  expectedRunID,
					CreationOperationID: "concurrent runner",
				})
				assert.NoErr(t, err)
				_, err = rb.Create(ctx, clMutator, pmNotifier, runNotifier)
				assert.ErrIsLike(t, err, `already created with OperationID "concurrent runner"`)
				assert.Loosely(t, StateChangedTag.In(err), should.BeFalse)
				assert.Loosely(t, transient.Tag.In(err), should.BeFalse)
			})

			t.Run("by us", func(t *ftt.Test) {
				err := datastore.Put(ctx, &run.Run{
					ID:                  expectedRunID,
					CreationOperationID: rb.OperationID,
				})
				assert.NoErr(t, err)
				r, err := rb.Create(ctx, clMutator, pmNotifier, runNotifier)
				assert.NoErr(t, err)
				assert.Loosely(t, r, should.NotBeNil)
			})
		})

		t.Run("ExpectedRunID works if CreateTime is given", func(t *ftt.Test) {
			rb.CreateTime = ct.Clock.Now()
			// For realism and to prevent non-determinism in production,
			// make CreateTime in the past.
			ct.Clock.Add(time.Second)
			assert.That(t, rb.ExpectedRunID(), should.Match(common.RunID(expectedRunID)))
		})

		t.Run("ExpectedRunID panics if CreateTime is not given", func(t *ftt.Test) {
			rb.CreateTime = time.Time{}
			assert.Loosely(t, func() { rb.ExpectedRunID() }, should.Panic)
		})

		t.Run("New Run is created", func(t *ftt.Test) {
			r, err := rb.Create(ctx, clMutator, pmNotifier, runNotifier)
			assert.NoErr(t, err)

			expectedRun := &run.Run{
				ID:         expectedRunID,
				EVersion:   1,
				CreateTime: datastore.RoundTime(ct.Clock.Now().UTC()),
				UpdateTime: datastore.RoundTime(ct.Clock.Now().UTC()),
				CLs:        common.CLIDs{cl1.ID, cl2.ID},
				RootCL:     cl1.ID,
				Status:     run.Status_PENDING,

				CreationOperationID: rb.run.CreationOperationID,
				ConfigGroupID:       rb.ConfigGroupID,
				Mode:                rb.Mode,
				Owner:               rb.Owner,
				CreatedBy:           rb.CreatedBy,
				BilledTo:            rb.BilledTo,
				Options:             &run.Options{},
				DepRuns:             common.RunIDs{"dead-beef"},
			}
			assert.That(t, r, should.Match(expectedRun))

			for i, cl := range rb.cls {
				assert.Loosely(t, cl.EVersion, should.Equal(rb.InputCLs[i].ExpectedEVersion+1))
				assert.That(t, cl.UpdateTime, should.Match(r.CreateTime))
			}

			// Run is properly saved
			saved := &run.Run{ID: expectedRun.ID}
			assert.NoErr(t, datastore.Get(ctx, saved))
			assert.That(t, saved, should.Match(expectedRun))

			for i := range rb.InputCLs {
				i := i
				t.Run(fmt.Sprintf("RunCL %d-th is properly saved", i), func(t *ftt.Test) {
					saved := &run.RunCL{
						ID:  rb.InputCLs[i].ID,
						Run: datastore.MakeKey(ctx, common.RunKind, expectedRunID),
					}
					assert.NoErr(t, datastore.Get(ctx, saved))
					assert.Loosely(t, saved.ExternalID, should.Equal(rb.cls[i].ExternalID))
					assert.That(t, saved.Trigger, should.Match(rb.InputCLs[i].TriggerInfo))
					assert.That(t, saved.Detail, should.Match(rb.cls[i].Snapshot))
				})
				t.Run(fmt.Sprintf("CL %d-th is properly updated", i), func(t *ftt.Test) {
					saved := &changelist.CL{ID: rb.InputCLs[i].ID}
					assert.NoErr(t, datastore.Get(ctx, saved))
					assert.Loosely(t, saved.IncompleteRuns.ContainsSorted(expectedRunID), should.BeTrue)
					assert.That(t, saved.UpdateTime, should.Match(expectedRun.UpdateTime))
					assert.Loosely(t, saved.EVersion, should.Equal(rb.InputCLs[i].ExpectedEVersion+1))
				})
			}

			// RunLog must contain the first entry for Creation.
			entries, err := run.LoadRunLogEntries(ctx, expectedRun.ID)
			assert.NoErr(t, err)
			assert.Loosely(t, entries, should.HaveLength(1))
			assert.That(t, entries[0].GetCreated().GetConfigGroupId(), should.Match(string(expectedRun.ConfigGroupID)))

			// Created metric is sent.
			assert.Loosely(t, ct.TSMonSentValue(ctx, metrics.Public.RunCreated, lProject, "cq-group", string(run.DryRun)), should.Equal(1))

			// Both PM and RM must be notified about new Run.
			pmtest.AssertInEventbox(t, ctx, lProject, &prjpb.Event{Event: &prjpb.Event_RunCreated{RunCreated: &prjpb.RunCreated{
				RunId: string(r.ID),
			}}})
			runtest.AssertInEventbox(t, ctx, r.ID, &eventpb.Event{Event: &eventpb.Event_Start{Start: &eventpb.Start{}}})
			// RM must have an immediate task to start working on a new Run.
			assert.That(t, runtest.Runs(ct.TQ.Tasks()), should.Match(common.RunIDs{r.ID}))
		})

		t.Run("Non standard run", func(t *ftt.Test) {
			rb.Mode = "CUSTOM_RUN"
			t.Run("Panic if mode definition is not provided", func(t *ftt.Test) {
				assert.Loosely(t, func() { rb.Create(ctx, clMutator, pmNotifier, runNotifier) }, should.Panic)
			})
			rb.ModeDefinition = &cfgpb.Mode{
				Name:            string(rb.Mode),
				CqLabelValue:    1,
				TriggeringLabel: "Custom",
				TriggeringValue: 1,
			}
			r, err := rb.Create(ctx, clMutator, pmNotifier, runNotifier)
			assert.NoErr(t, err)
			assert.Loosely(t, r.Mode, should.Equal(run.Mode("CUSTOM_RUN")))
			assert.That(t, r.ModeDefinition, should.Match(rb.ModeDefinition))
		})
	})
}
