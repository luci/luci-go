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
	"context"
	"testing"
	"time"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	cvbqpb "go.chromium.org/luci/cv/api/bigquery/v1"
	commonpb "go.chromium.org/luci/cv/api/common/v1"
	migrationpb "go.chromium.org/luci/cv/api/migration"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"
	gf "go.chromium.org/luci/cv/internal/gerrit/gerritfake"
	"go.chromium.org/luci/cv/internal/run"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
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

		m := MigrationServer{
			GFactory: ct.GFactory(),
		}
		req := &migrationpb.PostGerritMessageRequest{
			AttemptKey: "deadbeef",
			RunId:      "infra/123-1-deadbeef",
			Project:    "infra",

			Change:    1,
			Host:      "g-review.example.com",
			Revision:  "cqd-seen-revision",
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
				Status:        commonpb.Run_RUNNING,
				CreateTime:    ct.Clock.Now().Add(-time.Hour).UTC(),
			}
			So(datastore.Put(ctx, r), ShouldBeNil)

			// Without CL in CV.
			_, err = m.PostGerritMessage(ctx, req)
			So(grpcutil.Code(err), ShouldEqual, codes.Unavailable)

			// Put CL in CV (CL in Gerrit is faked separately below).
			ci := gf.CI(int(req.GetChange()))
			cl := changelist.MustGobID(req.GetHost(), req.GetChange()).MustCreateIfNotExists(ctx)
			cl.Snapshot = &changelist.Snapshot{
				ExternalUpdateTime:    ci.GetUpdated(),
				LuciProject:           req.GetProject(),
				MinEquivalentPatchset: 1,
				Patchset:              1,
				Kind:                  &changelist.Snapshot_Gerrit{Gerrit: &changelist.Gerrit{Info: ci}},
			}
			So(datastore.Put(ctx, cl), ShouldBeNil)

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

func TestReportVerifiedRun(t *testing.T) {
	t.Parallel()

	Convey("ReportVerifiedRun works", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		rnMock := runNotifierMock{}
		m := MigrationServer{RunNotifier: &rnMock}

		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity:             identity.Identity("project:infra"),
			PeerIdentityOverride: "user:cqdaemon@example.com",
		})

		const runID = common.RunID("infra/111-1-deadbeef")
		req := &migrationpb.ReportVerifiedRunRequest{
			// Id field is set in some tests below
			Run: &migrationpb.ReportedRun{
				Attempt: &cvbqpb.Attempt{
					Key:    runID.AttemptKey(),
					Status: cvbqpb.AttemptStatus_SUCCESS,
					// In practice, the other fields are also set by CQDaemon, but not
					// relevant in this test.
				},
			},
			FinalMessage: "meh",
			Action:       migrationpb.ReportVerifiedRunRequest_ACTION_SUBMIT,
		}

		loadVerifiedCQDRun := func() *VerifiedCQDRun {
			v := &VerifiedCQDRun{ID: runID}
			switch err := datastore.Get(ctx, v); {
			case err == datastore.ErrNoSuchEntity:
				return nil
			case err != nil:
				panic(err)
			default:
				return v
			}
		}

		Convey("without a Run in Datastore", func() {
			_, err := m.ReportVerifiedRun(ctx, req)
			So(grpcutil.Code(err), ShouldEqual, codes.NotFound)
			So(loadVerifiedCQDRun(), ShouldBeNil)
		})
		Convey("with a Run, always saves", func() {
			So(datastore.Put(ctx, &run.Run{
				ID:            runID,
				CQDAttemptKey: runID.AttemptKey(),
			}), ShouldBeNil)

			Convey("deduces the Run ID if not given and notifies Run Manager", func() {
				_, err := m.ReportVerifiedRun(ctx, req)
				So(err, ShouldBeNil)
				first := loadVerifiedCQDRun()
				So(first.Payload.Run.Attempt.Status, ShouldEqual, cvbqpb.AttemptStatus_SUCCESS)
				So(rnMock.verificationCompleted, ShouldContain, runID)

				Convey("does not overwrite existing data", func() {
					rnMock.verificationCompleted = nil
					req.Run.Id = string(runID)
					req.Run.Attempt.Status = cvbqpb.AttemptStatus_INFRA_FAILURE
					_, err := m.ReportVerifiedRun(ctx, req)
					So(err, ShouldBeNil)
					second := loadVerifiedCQDRun()
					So(second.UpdateTime, ShouldResemble, first.UpdateTime)
					So(second.Payload, ShouldResembleProto, first.Payload)
					// Run Manager doesn't need to be notified again.
					So(rnMock.verificationCompleted, ShouldBeEmpty)
				})
			})
		})
	})
}

func TestReportTryjobs(t *testing.T) {
	t.Parallel()

	Convey("ReportTryjobs works", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		rnMock := runNotifierMock{}
		m := MigrationServer{RunNotifier: &rnMock}

		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity:             identity.Identity("project:infra"),
			PeerIdentityOverride: "user:cqdaemon@example.com",
		})

		const runID = common.RunID("infra/111-1-deadbeef")
		req := &migrationpb.ReportTryjobsRequest{
			RunId: string(runID),
			Tryjobs: []*migrationpb.Tryjob{
				{
					Status: migrationpb.TryjobStatus_PENDING,
					Build:  &cvbqpb.Build{Id: 123, Origin: cvbqpb.Build_NOT_REUSED},
				},
				{
					Status: migrationpb.TryjobStatus_RUNNING,
					Build:  &cvbqpb.Build{Id: 124, Origin: cvbqpb.Build_REUSED},
				},
			},
		}

		_, err := m.ReportTryjobs(ctx, req)
		So(err, ShouldBeNil)

		ct.Clock.Add(time.Minute)
		req.Tryjobs[0].Status = migrationpb.TryjobStatus_RUNNING
		_, err = m.ReportTryjobs(ctx, req)
		So(err, ShouldBeNil)

		ct.Clock.Add(time.Minute)
		req.Tryjobs[1].Status = migrationpb.TryjobStatus_SUCCEEDED
		_, err = m.ReportTryjobs(ctx, req)
		So(err, ShouldBeNil)

		So(rnMock.tryjobsUpdated, ShouldResemble, common.RunIDs{runID, runID, runID})
		all, err := ListReportedTryjobs(ctx, runID, ct.Clock.Now().Add(-time.Hour), 0 /*unlimited*/)
		So(err, ShouldBeNil)
		So(all, ShouldHaveLength, 3)
	})
}

type runNotifierMock struct {
	verificationCompleted common.RunIDs
	tryjobsUpdated        common.RunIDs
	finished              common.RunIDs
}

func (r *runNotifierMock) NotifyCQDVerificationCompleted(ctx context.Context, runID common.RunID) error {
	r.verificationCompleted = append(r.verificationCompleted, runID)
	return nil
}

func (r *runNotifierMock) NotifyCQDTryjobsUpdated(ctx context.Context, runID common.RunID) error {
	r.tryjobsUpdated = append(r.tryjobsUpdated, runID)
	return nil
}

func (r *runNotifierMock) NotifyCQDFinished(ctx context.Context, runID common.RunID) error {
	r.finished = append(r.finished, runID)
	return nil
}
