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

package admin

import (
	"testing"
	"time"

	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/dsmapper"
	"go.chromium.org/luci/server/dsmapper/dsmapperpb"
	"go.chromium.org/luci/server/tq/tqtesting"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"
	gf "go.chromium.org/luci/cv/internal/gerrit/gerritfake"
	adminpb "go.chromium.org/luci/cv/internal/rpc/admin/api"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestUpgradeCLs(t *testing.T) {
	t.Parallel()

	Convey("Upgrade all CLs to contain metadata", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		const startEversion = 1
		var startTime = datastore.RoundTime(ct.Clock.Now().UTC())
		mkCL := func(id common.CLID, ci *gerritpb.ChangeInfo) (*changelist.CL, *datastore.Key) {
			cl := &changelist.CL{
				ID:         id,
				Snapshot:   nil,
				EVersion:   startEversion,
				UpdateTime: startTime,
			}
			if ci != nil {
				cl.Snapshot = &changelist.Snapshot{
					Kind: &changelist.Snapshot_Gerrit{Gerrit: &changelist.Gerrit{
						Info: ci,
					}},
					Metadata: nil,
				}
			}
			So(datastore.Put(ctx, cl), ShouldBeNil)
			k, err := datastore.KeyForObjErr(ctx, cl)
			So(err, ShouldBeNil)
			return cl, k
		}

		cl1, k1 := mkCL(1, gf.CI(1, gf.Desc("No meta at all")))
		cl2, k2 := mkCL(2, gf.CI(2, gf.Desc("TAG=true\n\nFooter: yes")))
		cl3, k3 := mkCL(3, gf.CI(3, gf.Desc("Intermediate state\n\nMigToMetadata: to be set")))
		cl3.Snapshot.Metadata = []*changelist.StringPair{{Key: "MigToMetadata", Value: "to be set"}}
		So(datastore.Put(ctx, cl3), ShouldBeNil)
		cl4, k4 := mkCL(4, nil)

		verify := func() {
			So(datastore.Get(ctx, cl1, cl2, cl3, cl4), ShouldBeNil)

			So(cl1.Snapshot.Metadata, ShouldBeEmpty)
			So(cl1.Snapshot.Metadata == nil, ShouldBeTrue)
			So(cl1.EVersion, ShouldEqual, startEversion+1)
			So(cl1.UpdateTime, ShouldHappenAfter, startTime)

			So(cl2.Snapshot.Metadata, ShouldResembleProto, []*changelist.StringPair{{Key: "Footer", Value: "yes"}, {Key: "TAG", Value: "true"}})
			So(cl2.EVersion, ShouldEqual, startEversion+1)
			So(cl2.UpdateTime, ShouldHappenAfter, startTime)

			So(cl3.Snapshot.Metadata, ShouldResembleProto, []*changelist.StringPair{{Key: "MigToMetadata", Value: "to be set"}})
			So(cl3.EVersion, ShouldEqual, startEversion) // unchanged
			So(cl3.UpdateTime, ShouldResemble, startTime)

			So(cl4.Snapshot, ShouldBeNil)
			So(cl4.UpdateTime, ShouldResemble, startTime)
		}

		Convey("Direct", func() {
			ct.Clock.Add(time.Minute)
			So(upgradeCLs(ctx, []*datastore.Key{k1, k2, k3, k4}), ShouldBeNil)
		})

		Convey("End to end", func() {
			ct.Clock.Add(time.Minute)
			ctrl := &dsmapper.Controller{}
			ctrl.Install(ct.TQDispatcher)
			a := New(ct.TQDispatcher, ctrl, nil, nil, nil)
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity:       "user:admin@example.com",
				IdentityGroups: []string{allowGroup},
			})

			jobID, err := a.DSMLaunchJob(ctx, &adminpb.DSMLaunchJobRequest{Name: "cl-metadata"})
			So(err, ShouldBeNil)
			ct.TQ.Run(ctx, tqtesting.StopWhenDrained())
			jobInfo, err := a.DSMGetJob(ctx, jobID)
			So(err, ShouldBeNil)
			So(jobInfo.GetInfo().GetState(), ShouldEqual, dsmapperpb.State_SUCCESS)

			verify()
		})
	})
}
