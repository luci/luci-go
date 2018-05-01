// Copyright 2018 The LUCI Authors.
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

package gerrit

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	. "github.com/smartystreets/goconvey/convey"

	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestCheckAccess(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	Convey("CheckAccess validate args", t, func() {
		srv, c := newMockPbClient(func(w http.ResponseWriter, r *http.Request) {})
		defer srv.Close()
		_, err := c.CheckAccess(ctx, &gerritpb.CheckAccessRequest{})
		So(err, ShouldErrLike, "project is required")

		_, err = c.CheckAccess(ctx, &gerritpb.CheckAccessRequest{Project: "p"})
		So(err, ShouldErrLike, "permission is required")

		_, err = c.CheckAccess(ctx, &gerritpb.CheckAccessRequest{Project: "p", Permission: "read"})
		So(err, ShouldErrLike, "account is required")
	})

	req := gerritpb.CheckAccessRequest{Project: "foo/bar", Permission: "read", Account: "a@example.com"}

	Convey("CheckAccess lack of ViewAccess permission", t, func() {
		srv, c := newMockPbClient(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(403)
		})
		defer srv.Close()
		_, err := c.CheckAccess(ctx, &req)
		s, ok := status.FromError(err)
		So(ok, ShouldBeTrue)
		So(s.Code(), ShouldEqual, codes.PermissionDenied)
	})

	Convey("CheckAccess happy path", t, func() {
		var status int
		srv, c := newMockPbClient(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(200)
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `)]}'{ "status": %d }`, status)
		})
		defer srv.Close()

		Convey("Allowed", func() {
			status = 200
			res, err := c.CheckAccess(ctx, &req)
			So(err, ShouldBeNil)
			So(res.Status, ShouldEqual, gerritpb.CheckAccessResponse_ALLOWED)
		})
		Convey("Forbidden", func() {
			status = 403
			res, err := c.CheckAccess(ctx, &req)
			So(err, ShouldBeNil)
			So(res.Status, ShouldEqual, gerritpb.CheckAccessResponse_FORBIDDEN)
		})
	})
}

func newMockPbClient(handler func(w http.ResponseWriter, r *http.Request)) (*httptest.Server, gerritpb.GerritClient) {
	// TODO(tandrii): rename this func once newMockClient name is no longer used in the same package.
	srv := httptest.NewServer(http.HandlerFunc(handler))
	return srv, &client{BaseURL: srv.URL}
}
