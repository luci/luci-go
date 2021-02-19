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

package diagnostic

import (
	"testing"
	"time"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	diagnosticpb "go.chromium.org/luci/cv/api/diagnostic"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/gerrit/poller"
	"go.chromium.org/luci/cv/internal/prjmanager"

	. "github.com/smartystreets/goconvey/convey"
)

func TestParseGerritURL(t *testing.T) {
	t.Parallel()

	Convey("parseGerritURL works", t, func() {
		eid, err := parseGerritURL("https://crrev.com/i/12/34")
		So(err, ShouldBeNil)
		So(string(eid), ShouldEqual, "gerrit/chrome-internal-review.googlesource.com/12")

		eid, err = parseGerritURL("https://crrev.com/c/12")
		So(err, ShouldBeNil)
		So(string(eid), ShouldEqual, "gerrit/chromium-review.googlesource.com/12")

		eid, err = parseGerritURL("https://chromium-review.googlesource.com/1541677")
		So(err, ShouldBeNil)
		So(string(eid), ShouldEqual, "gerrit/chromium-review.googlesource.com/1541677")

		eid, err = parseGerritURL("https://pdfium-review.googlesource.com/c/33")
		So(err, ShouldBeNil)
		So(string(eid), ShouldEqual, "gerrit/pdfium-review.googlesource.com/33")

		eid, err = parseGerritURL("https://chromium-review.googlesource.com/#/c/infra/luci/luci-go/+/1541677/7")
		So(err, ShouldBeNil)
		So(string(eid), ShouldEqual, "gerrit/chromium-review.googlesource.com/1541677")

		eid, err = parseGerritURL("https://chromium-review.googlesource.com/c/infra/luci/luci-go/+/2652967")
		So(err, ShouldBeNil)
		So(string(eid), ShouldEqual, "gerrit/chromium-review.googlesource.com/2652967")

		eid, err = parseGerritURL("chromium-review.googlesource.com/2652967")
		So(err, ShouldBeNil)
		So(string(eid), ShouldEqual, "gerrit/chromium-review.googlesource.com/2652967")
	})
}

func TestGetProject(t *testing.T) {
	t.Parallel()

	Convey("GetProject works", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		const lProject = "luci"
		d := DiagnosticServer{}

		Convey("without access", func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "anonymous:anonymous",
			})
			_, err := d.GetProject(ctx, &diagnosticpb.GetProjectRequest{Project: lProject})
			So(grpcutil.Code(err), ShouldEqual, codes.PermissionDenied)
		})

		Convey("with access", func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity:       "user:admin@example.com",
				IdentityGroups: []string{allowGroup},
			})
			Convey("not exists", func() {
				_, err := d.GetProject(ctx, &diagnosticpb.GetProjectRequest{Project: lProject})
				So(grpcutil.Code(err), ShouldEqual, codes.NotFound)
			})
		})
	})
}

func TestGetCL(t *testing.T) {
	t.Parallel()

	Convey("GetCL works", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		const lProject = "luci"
		d := DiagnosticServer{}

		Convey("without access", func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "anonymous:anonymous",
			})
			_, err := d.GetCL(ctx, &diagnosticpb.GetCLRequest{Id: 123})
			So(grpcutil.Code(err), ShouldEqual, codes.PermissionDenied)
		})

		Convey("with access", func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity:       "user:admin@example.com",
				IdentityGroups: []string{allowGroup},
			})
			So(datastore.Put(ctx, &changelist.CL{ID: 123, ExternalID: changelist.MustGobID("x-review", 44)}), ShouldBeNil)
			resp, err := d.GetCL(ctx, &diagnosticpb.GetCLRequest{Id: 123})
			So(err, ShouldBeNil)
			So(resp.GetExternalId(), ShouldEqual, "gerrit/x-review/44")
		})
	})
}

func TestGetPoller(t *testing.T) {
	t.Parallel()

	Convey("GetPoller works", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		const lProject = "luci"
		d := DiagnosticServer{}

		Convey("without access", func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "anonymous:anonymous",
			})
			_, err := d.GetPoller(ctx, &diagnosticpb.GetPollerRequest{Project: lProject})
			So(grpcutil.Code(err), ShouldEqual, codes.PermissionDenied)
		})

		Convey("with access", func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity:       "user:admin@example.com",
				IdentityGroups: []string{allowGroup},
			})
			now := ct.Clock.Now().UTC().Truncate(time.Second)
			So(datastore.Put(ctx, &poller.State{
				LuciProject: lProject,
				UpdateTime:  now,
			}), ShouldBeNil)
			resp, err := d.GetPoller(ctx, &diagnosticpb.GetPollerRequest{Project: lProject})
			So(err, ShouldBeNil)
			So(resp.GetUpdateTime().AsTime(), ShouldResemble, now)
		})
	})
}

func TestDeleteProjectEvents(t *testing.T) {
	t.Parallel()

	Convey("DeleteProjectEvents works", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		const lProject = "luci"
		d := DiagnosticServer{}

		Convey("without access", func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "anonymous:anonymous",
			})
			_, err := d.DeleteProjectEvents(ctx, &diagnosticpb.DeleteProjectEventsRequest{Project: lProject, Limit: 10})
			So(grpcutil.Code(err), ShouldEqual, codes.PermissionDenied)
		})

		Convey("with access", func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity:       "user:admin@example.com",
				IdentityGroups: []string{allowGroup},
			})

			So(prjmanager.NotifyCLUpdated(ctx, lProject, common.CLID(1), 1), ShouldBeNil)
			So(prjmanager.NotifyCLUpdated(ctx, lProject, common.CLID(2), 1), ShouldBeNil)
			So(prjmanager.UpdateConfig(ctx, lProject), ShouldBeNil)

			Convey("All", func() {
				resp, err := d.DeleteProjectEvents(ctx, &diagnosticpb.DeleteProjectEventsRequest{Project: lProject, Limit: 10})
				So(err, ShouldBeNil)
				So(resp.GetEvents(), ShouldResemble, map[string]int64{"*prjpb.Event_ClUpdated": 2, "*prjpb.Event_NewConfig": 1})
			})

			Convey("Limited", func() {
				resp, err := d.DeleteProjectEvents(ctx, &diagnosticpb.DeleteProjectEventsRequest{Project: lProject, Limit: 2})
				So(err, ShouldBeNil)
				sum := int64(0)
				for _, v := range resp.GetEvents() {
					sum += v
				}
				So(sum, ShouldEqual, 2)
			})
		})
	})
}
