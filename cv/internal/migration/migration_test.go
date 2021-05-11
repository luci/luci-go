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

package migration

import (
	"testing"
	"time"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	cvbqpb "go.chromium.org/luci/cv/api/bigquery/v1"
	migrationpb "go.chromium.org/luci/cv/api/migration"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"
	gf "go.chromium.org/luci/cv/internal/gerrit/gerritfake"
	"go.chromium.org/luci/cv/internal/run"

	. "github.com/smartystreets/goconvey/convey"
)

func TestClsOf(t *testing.T) {
	t.Parallel()

	Convey("clsOf works", t, func() {
		a := &cvbqpb.Attempt{}
		So(clsOf(a), ShouldEqual, "NO CLS")

		a.GerritChanges = []*cvbqpb.GerritChange{
			{Host: "abc", Change: 1, Patchset: 2},
		}
		So(clsOf(a), ShouldEqual, "1 CLs: [abc 1/2]")

		a.GerritChanges = []*cvbqpb.GerritChange{
			{Host: "abc", Change: 1, Patchset: 2},
			{Host: "abc", Change: 2, Patchset: 3},
		}
		So(clsOf(a), ShouldEqual, "2 CLs: [abc 1/2 2/3]")

		a.GerritChanges = []*cvbqpb.GerritChange{
			{Host: "abc", Change: 1, Patchset: 2},
			{Host: "xyz", Change: 2, Patchset: 3},
			{Host: "xyz", Change: 3, Patchset: 4},
			{Host: "abc", Change: 4, Patchset: 5},
			{Host: "abc", Change: 5, Patchset: 6},
		}
		So(clsOf(a), ShouldEqual, "5 CLs: [abc 1/2] [xyz 2/3 3/4] [abc 4/5 5/6]")
	})
}

func TestPostGerritMessage(t *testing.T) {
	t.Parallel()

	Convey("PostGerritMessage works", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		m := MigrationServer{}
		req := &migrationpb.PostGerritMessageRequest{
			AttemptKey: "deadbeef",
			RunId:      "infra/123-1-deadbeef",
			Project:    "infra",

			Change:    1,
			Host:      "g-review.example.com",
			Comment:   "some verification failed",
			SendEmail: true,
		}

		Convey("no permission to call CV", func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:hack-for-fun-and-profit@example.com",
			})
			_, err := m.PostGerritMessage(ctx, req)
			So(grpcutil.Code(err), ShouldEqual, codes.PermissionDenied)
		})

		Convey("with permission to call CV", func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity:             identity.Identity("project:infra"),
				PeerIdentityOverride: "user:cqdaemon@example.com",
			})

			// Without Run.
			_, err := m.PostGerritMessage(ctx, req)
			So(grpcutil.Code(err), ShouldEqual, codes.Unavailable)

			// Put a Run.
			r := &run.Run{
				ID:            common.RunID(req.GetRunId()),
				CQDAttemptKey: req.GetAttemptKey(),
				Mode:          run.DryRun,
				Status:        run.Status_RUNNING,
				CreateTime:    ct.Clock.Now().Add(-time.Hour).UTC(),
			}
			So(datastore.Put(ctx, r), ShouldBeNil)

			// Without CL in CV.
			_, err = m.PostGerritMessage(ctx, req)
			So(grpcutil.Code(err), ShouldEqual, codes.Unavailable)

			// Put CL in CV (CL in Gerrit is faked separately below).
			ci := gf.CI(int(req.GetChange()))
			cl, err := changelist.MustGobID(req.GetHost(), req.GetChange()).GetOrInsert(ctx, func(cl *changelist.CL) {
				cl.Snapshot = &changelist.Snapshot{
					ExternalUpdateTime:    ci.GetUpdated(),
					LuciProject:           req.GetProject(),
					MinEquivalentPatchset: 1,
					Patchset:              1,
					Kind:                  &changelist.Snapshot_Gerrit{Gerrit: &changelist.Gerrit{Info: ci}},
				}
			})
			So(err, ShouldBeNil)

			Convey("propagates Gerrit errors", func() {
				Convey("404", func() {
					ct.GFake.AddFrom(gf.WithCIs(req.GetHost(), gf.ACLRestricted("other-project"), ci))
					_, err = m.PostGerritMessage(ctx, req)
					So(grpcutil.Code(err), ShouldEqual, codes.NotFound)
				})
				Convey("403", func() {
					ct.GFake.AddFrom(gf.WithCIs(req.GetHost(), gf.ACLReadOnly(req.GetProject()), ci))
					_, err = m.PostGerritMessage(ctx, req)
					So(grpcutil.Code(err), ShouldEqual, codes.PermissionDenied)
				})
			})

			Convey("with Gerrit permission, posts to Gerrit", func() {
				ct.GFake.AddFrom(gf.WithCIs(req.GetHost(), gf.ACLPublic(), ci))
				mod := ci.GetUpdated().AsTime()

				_, err = m.PostGerritMessage(ctx, req)
				So(err, ShouldBeNil)

				ci2 := ct.GFake.GetChange(req.GetHost(), int(req.GetChange())).Info
				So(ci2, gf.ShouldLastMessageContain, req.GetComment())
				mod2 := ci2.GetUpdated().AsTime()
				So(mod2, ShouldHappenAfter, mod)

				Convey("but avoids duplication", func() {
					cl.Snapshot.GetGerrit().Info = ci2
					cl.Snapshot.ExternalUpdateTime = ci2.GetUpdated()
					So(datastore.Put(ctx, cl), ShouldBeNil)

					_, err = m.PostGerritMessage(ctx, req)
					So(err, ShouldBeNil)

					ci3 := ct.GFake.GetChange(req.GetHost(), int(req.GetChange())).Info
					mod3 := ci3.GetUpdated().AsTime()
					So(mod3, ShouldResemble, mod2)
				})

				Convey("posts a duplicate if prior message was before this Run started", func() {
					r.CreateTime = mod2.Add(time.Minute)
					So(datastore.Put(ctx, r), ShouldBeNil)

					ct.Clock.Set(r.CreateTime.Add(time.Hour))
					_, err = m.PostGerritMessage(ctx, req)
					So(err, ShouldBeNil)

					ci3 := ct.GFake.GetChange(req.GetHost(), int(req.GetChange())).Info
					mod3 := ci3.GetUpdated().AsTime()
					So(mod3, ShouldHappenAfter, r.CreateTime)
				})
			})
		})
	})
}
