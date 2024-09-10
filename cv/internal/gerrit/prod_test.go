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

package gerrit

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/errors"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestMakeClient(t *testing.T) {
	t.Parallel()

	Convey("With mocked token server", t, func() {
		epoch := datastore.RoundTime(testclock.TestRecentTimeUTC)
		ctx, tclock := testclock.UseTime(context.Background(), testclock.TestRecentTimeUTC)
		ctx = authtest.MockAuthConfig(ctx)

		const gHost = "first.example.com"

		f, err := newProd(ctx)
		So(err, ShouldBeNil)

		Convey("factory.token", func() {
			Convey("works", func() {
				f.mockMintProjectToken = func(context.Context, auth.ProjectTokenParams) (*auth.Token, error) {
					return &auth.Token{Token: "tok-1", Expiry: epoch.Add(2 * time.Minute)}, nil
				}
				tok, err := f.token(ctx, gHost, "lProject")
				So(err, ShouldBeNil)
				So(tok.AccessToken, ShouldEqual, "tok-1")
				So(tok.TokenType, ShouldEqual, "Bearer")
			})
			Convey("return errEmptyProjectToken when minted token is nil", func() {
				f.mockMintProjectToken = func(context.Context, auth.ProjectTokenParams) (*auth.Token, error) {
					return nil, nil
				}
				_, err := f.token(ctx, gHost, "lProject-1")
				So(err, ShouldErrLike, errEmptyProjectToken)
			})
			Convey("error", func() {
				f.mockMintProjectToken = func(context.Context, auth.ProjectTokenParams) (*auth.Token, error) {
					return nil, errors.New("flake")
				}
				_, err := f.token(ctx, gHost, "lProject")
				So(err, ShouldErrLike, "flake")
			})
		})

		Convey("factory.makeClient", func() {
			// This test calls ListChanges RPC because it is the easiest to mock empty
			// response for.
			var requests []*http.Request
			srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests = append(requests, r)
				w.Write([]byte(")]}'\n[]")) // no changes.
			}))
			defer srv.Close()
			f.baseTransport = srv.Client().Transport

			u, err := url.Parse(srv.URL)
			So(err, ShouldBeNil)

			limitedCtx, limitedCancel := context.WithTimeout(ctx, time.Minute)
			defer limitedCancel()

			Convey("project-scoped account", func() {
				tokenCnt := 0
				f.mockMintProjectToken = func(ctx context.Context, _ auth.ProjectTokenParams) (*auth.Token, error) {
					So(ctx.Err(), ShouldBeNil) // must not be expired.
					tokenCnt++
					return &auth.Token{
						Token:  fmt.Sprintf("tok-%d", tokenCnt),
						Expiry: epoch.Add(2 * time.Minute),
					}, nil
				}

				c, err := f.MakeClient(limitedCtx, u.Host, "lProject")
				So(err, ShouldBeNil)
				_, err = c.ListChanges(limitedCtx, &gerritpb.ListChangesRequest{})
				So(err, ShouldBeNil)
				So(requests, ShouldHaveLength, 1)
				So(requests[0].Header["Authorization"], ShouldResemble, []string{"Bearer tok-1"})

				// Ensure client can be used even if context of its creation expires.
				limitedCancel()
				So(limitedCtx.Err(), ShouldNotBeNil)
				// force token refresh
				tclock.Add(3 * time.Minute)
				_, err = c.ListChanges(ctx, &gerritpb.ListChangesRequest{})
				So(err, ShouldBeNil)
				So(requests, ShouldHaveLength, 2)
				So(requests[1].Header["Authorization"], ShouldResemble, []string{"Bearer tok-2"})
			})
		})
	})
}
