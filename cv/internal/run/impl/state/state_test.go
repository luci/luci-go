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
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/common/tree"
	"go.chromium.org/luci/cv/internal/configs/prjcfg"
	"go.chromium.org/luci/cv/internal/configs/prjcfg/prjcfgtest"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/run"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestCheckTree(t *testing.T) {
	t.Parallel()

	Convey("CheckTree", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp(t)
		defer cancel()
		const lProject = "chromium"
		rs := &RunState{
			Run: run.Run{
				ID:         common.MakeRunID(lProject, ct.Clock.Now().Add(-2*time.Minute), 1, []byte("deadbeef")),
				Submission: &run.Submission{},
			},
		}
		prjcfgtest.Create(ctx, lProject, &cfgpb.Config{
			ConfigGroups: []*cfgpb.ConfigGroup{
				{
					Name: "main",
					Verifiers: &cfgpb.Verifiers{
						TreeStatus: &cfgpb.Verifiers_TreeStatus{
							Url: "https://tree-status.appspot.com",
						},
					},
				},
			},
		})
		meta, err := prjcfg.GetLatestMeta(ctx, lProject)
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

		Convey("Closed", func() {
			ct.TreeFake.ModifyState(ctx, tree.Closed)
			open, err := rs.CheckTree(ctx, client)
			So(err, ShouldBeNil)
			So(open, ShouldBeFalse)
			So(rs.Run.Submission.TreeOpen, ShouldBeFalse)
			So(rs.Run.Submission.LastTreeCheckTime, ShouldResembleProto, timestamppb.New(ct.Clock.Now().UTC()))
		})

		Convey("Closed but ignored", func() {
			ct.TreeFake.ModifyState(ctx, tree.Closed)
			rs.Run.Options = &run.Options{SkipTreeChecks: true}
			open, err := rs.CheckTree(ctx, client)
			So(err, ShouldBeNil)
			So(open, ShouldBeTrue)
			So(rs.Run.Submission.TreeOpen, ShouldBeTrue)
			So(rs.Run.Submission.LastTreeCheckTime, ShouldResembleProto, timestamppb.New(ct.Clock.Now().UTC()))
		})

		Convey("Tree not defined", func() {
			ct.TreeFake.ModifyState(ctx, tree.Closed)
			prjcfgtest.Update(ctx, lProject, &cfgpb.Config{
				ConfigGroups: []*cfgpb.ConfigGroup{
					{Name: "main"},
					// No Tree defined
				},
			})
			meta, err := prjcfg.GetLatestMeta(ctx, lProject)
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
