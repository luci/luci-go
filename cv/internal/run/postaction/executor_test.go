// Copyright 2023 The LUCI Authors.
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

package postaction

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/errors"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/gae/service/datastore"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/configs/prjcfg/prjcfgtest"
	"go.chromium.org/luci/cv/internal/configs/validation"
	"go.chromium.org/luci/cv/internal/cvtesting"
	gf "go.chromium.org/luci/cv/internal/gerrit/gerritfake"
	"go.chromium.org/luci/cv/internal/run"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestExecutePostActionOp(t *testing.T) {
	t.Parallel()

	Convey("Do", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp(t)
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
				Run:        datastore.MakeKey(ctx, common.RunKind, string(runID)),
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

		makeRunWithCLs := func(cis ...*gerritpb.ChangeInfo) *run.Run {
			if len(cis) == 0 {
				panic(fmt.Errorf("at least one CL required"))
			}
			r := &run.Run{
				ID:            runID,
				Status:        run.Status_SUCCEEDED,
				Mode:          run.FullRun,
				ConfigGroupID: prjcfgtest.MustExist(ctx, lProject).ConfigGroupIDs[0],
			}

			for _, ci := range cis {
				_, rcl := ensureCL(ci)
				r.CLs = append(r.CLs, rcl.ID)
			}
			So(datastore.Put(ctx, r), ShouldBeNil)
			return r
		}

		postActionCfg := &cfgpb.ConfigGroup_PostAction{
			Name: "label-vote",
			Action: &cfgpb.ConfigGroup_PostAction_VoteGerritLabels_{
				VoteGerritLabels: &cfgpb.ConfigGroup_PostAction_VoteGerritLabels{},
			},
		}
		configPostVote := func(n string, v int32) {
			postActionCfg.GetVoteGerritLabels().Votes = append(
				postActionCfg.GetVoteGerritLabels().Votes,
				&cfgpb.ConfigGroup_PostAction_VoteGerritLabels_Vote{Name: n, Value: v})
		}
		newExecutor := func(ctx context.Context, r *run.Run) *Executor {
			return &Executor{
				GFactory:          ct.GFactory(),
				Run:               r,
				IsCancelRequested: func() bool { return false },
				Payload: &run.OngoingLongOps_Op_ExecutePostActionPayload{
					Name: postActionCfg.GetName(),
					Kind: &run.OngoingLongOps_Op_ExecutePostActionPayload_ConfigAction{
						ConfigAction: postActionCfg,
					},
				},
			}
		}
		listLabels := func(clNum int) map[string]int32 {
			ret := map[string]int32{}
			info := ct.GFake.GetChange(gHost, clNum).Info
			for n, l := range info.Labels {
				ret[n] = l.All[0].Value
			}
			return ret
		}

		Convey("votes labels", func() {
			var summary string
			var err error
			configPostVote("label-1", 2)
			configPostVote("label-2", 0)

			Convey("adds new labels", func() {
				exe := newExecutor(ctx, makeRunWithCLs(gf.CI(gChange1)))
				summary, err = exe.Do(ctx)
				So(err, ShouldBeNil)
				So(listLabels(gChange1), ShouldResemble, map[string]int32{
					"label-1": 2,
					"label-2": 0,
				})
			})

			Convey("leaves other labels as they are", func() {
				exe := newExecutor(ctx, makeRunWithCLs(gf.CI(gChange1, gf.Vote("label-3", 1))))
				summary, err = exe.Do(ctx)
				So(err, ShouldBeNil)
				So(listLabels(gChange1), ShouldResemble, map[string]int32{
					"label-1": 2,
					"label-2": 0,
					"label-3": 1,
				})
			})

			Convey("overrides the values if a given label already exists", func() {
				exe := newExecutor(ctx, makeRunWithCLs(gf.CI(gChange1, gf.Vote("label-1", -1))))
				summary, err = exe.Do(ctx)
				So(err, ShouldBeNil)
				So(listLabels(gChange1), ShouldResemble, map[string]int32{
					"label-1": 2,
					"label-2": 0,
				})
			})

			Convey("multi CL run", func() {
				exe := newExecutor(ctx, makeRunWithCLs(gf.CI(gChange1), gf.CI(gChange2)))
				summary, err = exe.Do(ctx)
				So(err, ShouldBeNil)
				So(listLabels(gChange1), ShouldResemble, map[string]int32{
					"label-1": 2,
					"label-2": 0,
				})
				So(listLabels(gChange2), ShouldResemble, map[string]int32{
					"label-1": 2,
					"label-2": 0,
				})
			})
			So(summary, ShouldEqual, "all votes succeeded")
		})

		Convey("cancel if requested", func() {
			exe := newExecutor(ctx, makeRunWithCLs(gf.CI(gChange1)))
			configPostVote("label-1", 2)
			ctx, cancel = context.WithTimeout(ctx, 10*time.Second)
			defer cancel()

			Convey("before the execution started", func() {
				exe.IsCancelRequested = func() bool { return true }
				summary, err := exe.Do(ctx)
				So(err, ShouldErrLike, "CancelRequested")
				So(summary, ShouldEqual, "cancellation has been requested before the post action starts")
			})
			Convey("after the execution started", func() {
				doErr := make(chan error)
				var lck sync.Mutex
				exe.testBeforeCLMutation = func(ctx context.Context, rcl *run.RunCL, req *gerritpb.SetReviewRequest) {
					lck.Lock()
					exe.IsCancelRequested = func() bool { return true }
					lck.Unlock()
				}
				go func() {
					_, err := exe.Do(ctx)
					doErr <- err
					close(doErr)
				}()
				select {
				case err := <-doErr:
					So(err, ShouldErrLike, "CL mutation aborted due to op cancellation")
				case <-ctx.Done():
					panic("mutation didn't start within 10 secs")
				}
			})
		})

		Convey("setVoteSummary", func() {
			exe := newExecutor(ctx, makeRunWithCLs(gf.CI(gChange1)))
			newRCLs := func(n int) []*run.RunCL {
				var ret []*run.RunCL
				for i := 0; i < n; i++ {
					ret = append(ret, &run.RunCL{
						ExternalID: changelist.MustGobID("example.com", int64(i+1)),
					})
				}
				return ret
			}
			permErr := errors.New("permanently failed")
			rcls := newRCLs(4)
			errs := errors.NewMultiError(nil, nil, nil, nil)

			Convey("all succeeded/failed/cancelled", func() {
				So(exe.voteSummary(ctx, rcls, errs), ShouldEqual, "all votes succeeded")
				errs = errors.NewMultiError(permErr, permErr, permErr, permErr)
				So(exe.voteSummary(ctx, rcls, errs), ShouldEqual, "all votes failed")
				errs = errors.NewMultiError(errOpCancel, errOpCancel, errOpCancel, errOpCancel)
				So(exe.voteSummary(ctx, rcls, errs), ShouldEqual, "all votes cancelled")
			})
			Convey("a mix of succeeded/failed/cancelled", func() {
				// mix of all
				errs[1], errs[2] = permErr, errOpCancel
				So(exe.voteSummary(ctx, rcls, errs), ShouldEqual,
					strings.Join([]string{
						"Results for Gerrit label votes",
						"- succeeded: https://example.com/c/1, https://example.com/c/4",
						"- failed: https://example.com/c/2",
						"- cancelled: https://example.com/c/3",
					}, "\n"),
				)
			})
		})

		Convey("if FailedPrecondition", func() {
			// fake the ACL to return FailedPrecondition always for the CL.
			ci := gf.CI(gChange1)
			run := makeRunWithCLs(ci)
			fakeResponseStatus := func(_ gf.Operation, _ string) *status.Status {
				return status.New(codes.FailedPrecondition, "error")
			}
			ct.GFake.MutateChange(gHost, int(ci.GetNumber()), func(c *gf.Change) {
				c.ACLs = fakeResponseStatus
			})

			// ensure that the execution fails and no labels are voted.
			configPostVote("label-1", 2)
			exe := newExecutor(ctx, run)
			_, err := exe.Do(ctx)
			So(err, ShouldErrLike, "FailedPrecondition")
			So(listLabels(gChange1), ShouldResemble, map[string]int32{})

			Convey("skip the post action, if the CL abandoned", func() {
				// mark the CL as abandoned.
				cl := &changelist.CL{ID: run.CLs[0]}
				So(datastore.Get(ctx, cl), ShouldBeNil)
				cl.Snapshot.GetGerrit().GetInfo().Status = gerritpb.ChangeStatus_ABANDONED
				cl.EVersion++
				So(datastore.Put(ctx, cl), ShouldBeNil)

				// give it another try
				_, err := exe.Do(ctx)
				So(err, ShouldBeNil)
				// No vote should have been performed. (It can't be, anyways)
				So(listLabels(gChange1), ShouldResemble, map[string]int32{})
			})

			Convey("skip the post action, if the CL submitted", func() {
				// mark the CL as abandoned.
				cl := &changelist.CL{ID: run.CLs[0]}
				So(datastore.Get(ctx, cl), ShouldBeNil)
				cl.Snapshot.GetGerrit().GetInfo().Status = gerritpb.ChangeStatus_MERGED
				cl.EVersion++
				So(datastore.Put(ctx, cl), ShouldBeNil)

				// give it another try
				_, err := exe.Do(ctx)
				So(err, ShouldBeNil)
				// No vote should have been performed. (It can't be, anyways)
				So(listLabels(gChange1), ShouldResemble, map[string]int32{})
			})
		})
	})
}
