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

package state

import (
	"context"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/gae/service/datastore"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/config"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/tree"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestRemoveRunFromCLs(t *testing.T) {
	t.Parallel()

	Convey("RemoveRunFromCLs", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()
		s := &RunState{
			Run: run.Run{
				ID:  common.RunID("chromium/111-2-deadbeef"),
				CLs: common.CLIDs{1},
			},
		}
		Convey("Works", func() {
			err := datastore.Put(ctx, &changelist.CL{
				ID:             1,
				IncompleteRuns: common.MakeRunIDs("chromium/111-2-deadbeef", "infra/999-2-cafecafe"),
				EVersion:       3,
				UpdateTime:     clock.Now(ctx).UTC(),
			})
			So(err, ShouldBeNil)

			ct.Clock.Add(1 * time.Hour)
			now := clock.Now(ctx).UTC()
			err = datastore.RunInTransaction(ctx, func(ctx context.Context) error {
				return s.RemoveRunFromCLs(ctx)
			}, nil)
			So(err, ShouldBeNil)

			cl := changelist.CL{ID: 1}
			So(datastore.Get(ctx, &cl), ShouldBeNil)
			So(cl, ShouldResemble, changelist.CL{
				ID:             1,
				IncompleteRuns: common.MakeRunIDs("infra/999-2-cafecafe"),
				EVersion:       4,
				UpdateTime:     now,
			})
		})
	})
}

func TestCheckTree(t *testing.T) {
	t.Parallel()

	Convey("CheckTree", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()
		const lProject = "chromium"
		rs := &RunState{
			Run: run.Run{
				ID:         common.MakeRunID(lProject, ct.Clock.Now().Add(-2*time.Minute), 1, []byte("deadbeef")),
				Submission: &run.Submission{},
			},
		}
		ct.Cfg.Create(ctx, lProject, &cfgpb.Config{
			ConfigGroups: []*cfgpb.ConfigGroup{
				{
					Name: "main",
					Verifiers: &cfgpb.Verifiers{
						TreeStatus: &cfgpb.Verifiers_TreeStatus{
							Url: "tree.example.com",
						},
					},
				},
			},
		})
		meta, err := config.GetLatestMeta(ctx, lProject)
		So(err, ShouldBeNil)
		So(meta.ConfigGroupIDs, ShouldHaveLength, 1)
		rs.Run.ConfigGroupID = meta.ConfigGroupIDs[0]
		client := ct.TreeFake.Client()

		Convey("Open", func() {
			ct.TreeFake.ModifyState(ctx, tree.Open)
			open, err := rs.CheckTree(ctx, client)
			So(err, ShouldBeNil)
			So(open, ShouldBeTrue)
			So(rs.Run.Submission.TreeOpen, ShouldBeTrue)
			So(rs.Run.Submission.LastTreeCheckTime, ShouldResembleProto, timestamppb.New(ct.Clock.Now().UTC()))
		})

		Convey("Close", func() {
			ct.TreeFake.ModifyState(ctx, tree.Closed)
			open, err := rs.CheckTree(ctx, client)
			So(err, ShouldBeNil)
			So(open, ShouldBeFalse)
			So(rs.Run.Submission.TreeOpen, ShouldBeFalse)
			So(rs.Run.Submission.LastTreeCheckTime, ShouldResembleProto, timestamppb.New(ct.Clock.Now().UTC()))
		})

		Convey("Tree not defined", func() {
			ct.TreeFake.ModifyState(ctx, tree.Closed)
			ct.Cfg.Update(ctx, lProject, &cfgpb.Config{
				ConfigGroups: []*cfgpb.ConfigGroup{
					{Name: "main"},
					// No Tree defined
				},
			})
			meta, err := config.GetLatestMeta(ctx, lProject)
			So(err, ShouldBeNil)
			So(meta.ConfigGroupIDs, ShouldHaveLength, 1)
			rs.Run.ConfigGroupID = meta.ConfigGroupIDs[0]
			open, err := rs.CheckTree(ctx, client)
			So(err, ShouldBeNil)
			So(open, ShouldBeTrue)
			So(rs.Run.Submission.TreeOpen, ShouldBeTrue)
			So(rs.Run.Submission.LastTreeCheckTime, ShouldResembleProto, timestamppb.New(ct.Clock.Now().UTC()))
		})
	})
}
