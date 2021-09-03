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

package rpc

import (
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	rpcpb "go.chromium.org/luci/cv/api/rpc/v0"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/run"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestGetRun(t *testing.T) {
	t.Parallel()

	Convey("GetRun", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		rs := RunsServer{}
		rid := common.RunID("prj/123-deadbeef")
		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity:       "user:admin@example.com",
			IdentityGroups: []string{allowGroup},
		})

		Convey("w/o access", func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "anonymous:anonymous",
			})
			_, err := rs.GetRun(ctx, &rpcpb.GetRunRequest{Id: rid.PublicID()})
			So(grpcutil.Code(err), ShouldEqual, codes.PermissionDenied)
		})

		Convey("w/ an invalid public ID", func() {
			_, err := rs.GetRun(ctx, &rpcpb.GetRunRequest{Id: "something valid"})
			So(grpcutil.Code(err), ShouldEqual, codes.InvalidArgument)
		})

		Convey("w/ an non-existing Run ID", func() {
			_, err := rs.GetRun(ctx, &rpcpb.GetRunRequest{Id: rid.PublicID()})
			So(grpcutil.Code(err), ShouldEqual, codes.NotFound)
		})

		Convey("w/ an existing RunID", func() {
			const gHost = "r-review.example.com"
			cl1 := changelist.MustGobID(gHost, 1).MustCreateIfNotExists(ctx)
			cl2 := changelist.MustGobID(gHost, 2).MustCreateIfNotExists(ctx)
			epoch := testclock.TestRecentTimeUTC.Truncate(time.Millisecond)
			So(datastore.Put(
				ctx,
				&run.Run{
					ID:         rid,
					CreateTime: epoch,
					StartTime:  epoch.Add(time.Second),
					UpdateTime: epoch.Add(time.Minute),
					EndTime:    epoch.Add(time.Hour),
					Owner:      "user:foo@example.org",
					CLs:        common.MakeCLIDs(1, 2),
				},
				&run.RunCL{
					Run: datastore.MakeKey(ctx, run.RunKind, string(rid)),
					ID:  cl1.ID, IndexedID: cl1.ID,
					ExternalID: cl1.ExternalID,
					Detail: &changelist.Snapshot{
						Patchset: 39,
					},
				},
				&run.RunCL{
					Run: datastore.MakeKey(ctx, run.RunKind, string(rid)),
					ID:  cl2.ID, IndexedID: cl2.ID,
					ExternalID: cl2.ExternalID,
					Detail: &changelist.Snapshot{
						Patchset: 40,
					},
				},
			), ShouldBeNil)

			resp, err := rs.GetRun(ctx, &rpcpb.GetRunRequest{Id: rid.PublicID()})
			So(grpcutil.Code(err), ShouldEqual, codes.OK)
			So(resp, ShouldResembleProto, &rpcpb.Run{
				Id:         rid.PublicID(),
				CreateTime: timestamppb.New(epoch),
				StartTime:  timestamppb.New(epoch.Add(time.Second)),
				UpdateTime: timestamppb.New(epoch.Add(time.Minute)),
				EndTime:    timestamppb.New(epoch.Add(time.Hour)),
				Owner:      "user:foo@example.org",
				Cls: []*rpcpb.GerritChange{
					{Host: gHost, Change: 1, Patchset: 39},
					{Host: gHost, Change: 2, Patchset: 40},
				},
			})
		})
	})
}
