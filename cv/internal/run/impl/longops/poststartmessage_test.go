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

package longops

import (
	"fmt"
	"testing"
	"time"

	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/configs/prjcfg/prjcfgtest"
	"go.chromium.org/luci/cv/internal/configs/validation"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/gerrit/botdata"
	gf "go.chromium.org/luci/cv/internal/gerrit/gerritfake"
	"go.chromium.org/luci/cv/internal/gerrit/trigger"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/eventpb"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestPostStartMessage(t *testing.T) {
	t.Parallel()

	Convey("PostStartMessageOp works", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		const (
			lProject = "chromeos"
			runID    = lProject + "/777-1-deadbeef"
			gHost    = "g-review.example.com"
			gChange1 = 111
			gChange2 = 222
		)

		cfg := cfgpb.Config{
			CqStatusHost: validation.CQStatusHostPublic,
			ConfigGroups: []*cfgpb.ConfigGroup{
				{Name: "test"},
			},
		}
		prjcfgtest.Create(ctx, lProject, &cfg)

		ensureCL := func(ci *gerritpb.ChangeInfo) (*changelist.CL, *run.RunCL) {
			t := trigger.Find(ci, cfg.GetConfigGroups()[0])
			if t == nil {
				panic(fmt.Errorf("CL %d must be triggered", ci.GetNumber()))
			}

			if ct.GFake.Has(gHost, int(ci.GetNumber())) {
				ct.GFake.MutateChange(gHost, int(ci.GetNumber()), func(c *gf.Change) {
					c.Info = ci
				})
			} else {
				ct.GFake.AddFrom(gf.WithCIs(gHost, gf.ACLRestricted(lProject), ci))
			}

			cl := changelist.MustGobID(gHost, ci.GetNumber()).MustCreateIfNotExists(ctx)
			rcl := &run.RunCL{
				ID:         cl.ID,
				ExternalID: cl.ExternalID,
				IndexedID:  cl.ID,
				Trigger:    t,
				Run:        datastore.MakeKey(ctx, run.RunKind, string(runID)),
				Detail: &changelist.Snapshot{
					Kind: &changelist.Snapshot_Gerrit{Gerrit: &changelist.Gerrit{
						Host: gHost,
						Info: ci,
					}},
					ExternalUpdateTime: timestamppb.New(ct.Clock.Now()),
				},
			}
			cl.Snapshot = rcl.Detail
			cl.EVersion++
			So(datastore.Put(ctx, cl, rcl), ShouldBeNil)
			return cl, rcl
		}

		clidOf := func(gChange int64) int64 {
			cl := changelist.MustGobID(gHost, gChange).MustCreateIfNotExists(ctx)
			return int64(cl.ID)
		}

		clidsOf := func(gChanges ...int64) []int64 {
			out := make([]int64, len(gChanges))
			for i, gChange := range gChanges {
				out[i] = clidOf(gChange)
			}
			return out
		}

		makeRunWithCLs := func(r *run.Run, cis ...*gerritpb.ChangeInfo) *run.Run {
			if len(cis) == 0 {
				panic(fmt.Errorf("at least one CL required"))
			}
			if r == nil {
				r = &run.Run{}
			}
			r.ID = runID
			r.Status = run.Status_RUNNING
			for _, ci := range cis {
				_, rcl := ensureCL(ci)
				r.CLs = append(r.CLs, rcl.ID)
			}
			if r.Mode == "" {
				r.Mode = run.FullRun
			}
			if r.ConfigGroupID == "" {
				r.ConfigGroupID = prjcfgtest.MustExist(ctx, lProject).ConfigGroupIDs[0]
			}
			So(datastore.Put(ctx, r), ShouldBeNil)
			return r
		}

		makeOp := func(r *run.Run) *PostStartMessageOp {
			return &PostStartMessageOp{
				Base: &Base{
					Op: &run.OngoingLongOps_Op{
						Deadline:        timestamppb.New(ct.Clock.Now().Add(10000 * time.Hour)),
						CancelRequested: false,
						Work: &run.OngoingLongOps_Op_PostStartMessage{
							PostStartMessage: true,
						},
					},
					IsCancelRequested: func() bool { return false },
					Run:               r,
				},
				Env:      ct.Env,
				GFactory: ct.GFactory(),
			}
		}

		Convey("Happy path without status URL", func() {
			cfg.CqStatusHost = ""
			prjcfgtest.Update(ctx, lProject, &cfg)

			op := makeOp(makeRunWithCLs(nil, gf.CI(gChange1, gf.CQ(+2))))
			res, err := op.Do(ctx)
			So(err, ShouldBeNil)
			So(res.GetStatus(), ShouldEqual, eventpb.LongOpCompleted_SUCCEEDED)
			So(res.GetPostStartMessage(), ShouldResembleProto, &eventpb.LongOpCompleted_PostStartMessage{
				Posted: clidsOf(gChange1),
			})

			So(ct.GFake.GetChange(gHost, gChange1).Info, gf.ShouldLastMessageContain, "CV is trying the patch.\n\nBot data: ")
		})

		Convey("Happy path", func() {
			op := makeOp(makeRunWithCLs(nil, gf.CI(gChange1, gf.CQ(+2))))
			res, err := op.Do(ctx)
			So(err, ShouldBeNil)
			So(res.GetStatus(), ShouldEqual, eventpb.LongOpCompleted_SUCCEEDED)
			So(res.GetPostStartMessage(), ShouldResembleProto, &eventpb.LongOpCompleted_PostStartMessage{
				Posted: clidsOf(gChange1),
			})

			ci := ct.GFake.GetChange(gHost, gChange1).Info
			So(ci, gf.ShouldLastMessageContain, "CV is trying the patch.\n\nFollow status at:")
			So(ci, gf.ShouldLastMessageContain, "https://luci-change-verifier.appspot.com/ui/run/chromeos/777-1-deadbeef")
			So(ci, gf.ShouldLastMessageContain, "Bot data:")
			// Should post exactly one message.
			So(ci.GetMessages(), ShouldHaveLength, 1)
		})

		Convey("Happy path with multiple CLs", func() {
			op := makeOp(makeRunWithCLs(
				&run.Run{Mode: run.DryRun},
				gf.CI(gChange1, gf.CQ(+1)),
				gf.CI(gChange2, gf.CQ(+1)),
			))
			res, err := op.Do(ctx)
			So(err, ShouldBeNil)
			So(res.GetStatus(), ShouldEqual, eventpb.LongOpCompleted_SUCCEEDED)
			So(res.GetPostStartMessage(), ShouldResembleProto, &eventpb.LongOpCompleted_PostStartMessage{
				Posted: clidsOf(gChange1, gChange2),
			})

			for _, gChange := range []int{gChange1, gChange2} {
				ci := ct.GFake.GetChange(gHost, gChange).Info
				So(ci, gf.ShouldLastMessageContain, "Dry run: CV is trying the patch.\n\nFollow status at:")
				// Should post exactly one message.
				So(ci.GetMessages(), ShouldHaveLength, 1)
				bd, ok := botdata.Parse(ci.GetMessages()[0])
				So(ok, ShouldBeTrue)
				So(bd.Action, ShouldEqual, botdata.Start)
				So(bd.CLs, ShouldHaveLength, 2)
				So(bd.CLs[0].Number, ShouldEqual, gChange1)
				So(bd.CLs[1].Number, ShouldEqual, gChange2)
			}
		})

		Convey("Best effort avoidance of duplicated messages", func() {
			// Make two same PostStartMessageOp objects, since they are single-use
			// only.
			opFirst := makeOp(makeRunWithCLs(nil, gf.CI(gChange1, gf.CQ(+2))))
			opRetry := makeOp(makeRunWithCLs(nil, gf.CI(gChange1, gf.CQ(+2))))

			// Simulate first try updating Gerrit, but somehow crashing before getting
			// response from Gerrit.
			_, err := opFirst.Do(ctx)
			So(err, ShouldBeNil)
			ci := ct.GFake.GetChange(gHost, gChange1).Info
			So(ci, gf.ShouldLastMessageContain, "CV is trying the patch")
			So(ci.GetMessages(), ShouldHaveLength, 1)

			Convey("very quick retry leads to dups", func() {
				ct.Clock.Add(time.Second)
				res, err := opRetry.Do(ctx)
				So(err, ShouldBeNil)
				So(res.GetStatus(), ShouldEqual, eventpb.LongOpCompleted_SUCCEEDED)
				So(res.GetPostStartMessage(), ShouldResembleProto, &eventpb.LongOpCompleted_PostStartMessage{
					Posted: clidsOf(gChange1),
				})
				So(ct.GFake.GetChange(gHost, gChange1).Info.GetMessages(), ShouldHaveLength, 2)
			})

			Convey("later retry", func() {
				ct.Clock.Add(staleCLAgeThreshold)
				res, err := opRetry.Do(ctx)
				So(err, ShouldBeNil)
				So(res.GetStatus(), ShouldEqual, eventpb.LongOpCompleted_SUCCEEDED)
				So(res.GetPostStartMessage(), ShouldResembleProto, &eventpb.LongOpCompleted_PostStartMessage{
					Posted: clidsOf(gChange1),
				})
				// There should still be exactly 1 message.
				So(ct.GFake.GetChange(gHost, gChange1).Info.GetMessages(), ShouldHaveLength, 1)
			})
		})

		Convey("Failures", func() {
			op := makeOp(makeRunWithCLs(
				&run.Run{Mode: run.DryRun},
				gf.CI(gChange1, gf.CQ(+1)),
				gf.CI(gChange2, gf.CQ(+1)),
			))

			Convey("PermissionDenied is fatal failure", func() {
				ct.GFake.MutateChange(gHost, gChange1, func(c *gf.Change) {
					c.ACLs = func(op gf.Operation, luciProject string) *status.Status {
						return status.New(codes.PermissionDenied, "admin-is-angry-today")
					}
				})
				res, err := op.Do(ctx)
				So(err, ShouldNotBeNil)
				So(transient.Tag.In(err), ShouldBeFalse)
				So(res.GetStatus(), ShouldEqual, eventpb.LongOpCompleted_FAILED)
				p := res.GetPostStartMessage()
				So(p.GetPermanentErrors(), ShouldHaveLength, 1)
				So(p.GetPermanentErrors()[clidOf(gChange1)], ShouldContainSubstring, "admin-is-angry-today")
				So(p.GetPosted(), ShouldResemble, clidsOf(gChange2))
			})

			Convey("Gerrit internal error is deemed transient", func() {
				ct.GFake.MutateChange(gHost, gChange2, func(c *gf.Change) {
					// HACK: simulate HTTP 5xx on SetReview.
					c.ACLs = func(op gf.Operation, luciProject string) *status.Status {
						return status.New(codes.Internal, "oops, temp error")
					}
				})
				res, err := op.Do(ctx)
				So(err, ShouldErrLike, "oops, temp error")
				So(transient.Tag.In(err), ShouldBeTrue)
				So(res, ShouldBeNil)
			})

			Convey("Fatal failures dominate transient", func() {
				ct.GFake.MutateChange(gHost, gChange1, func(c *gf.Change) {
					c.ACLs = func(op gf.Operation, luciProject string) *status.Status {
						return status.New(codes.PermissionDenied, "admin-is-angry-today")
					}
				})
				ct.GFake.MutateChange(gHost, gChange2, func(c *gf.Change) {
					// HACK: simulate HTTP 5xx on SetReview.
					c.ACLs = func(op gf.Operation, luciProject string) *status.Status {
						return status.New(codes.Internal, "oops, temp error")
					}
				})
				res, err := op.Do(ctx)
				So(err, ShouldNotBeNil)
				So(transient.Tag.In(err), ShouldBeFalse)
				So(res.GetStatus(), ShouldEqual, eventpb.LongOpCompleted_FAILED)
				p := res.GetPostStartMessage()
				So(p.GetPermanentErrors(), ShouldHaveLength, 1)
				So(p.GetPermanentErrors()[clidOf(gChange1)], ShouldContainSubstring, "admin-is-angry-today")
				So(p.GetPosted(), ShouldBeEmpty)
			})
		})
	})
}
