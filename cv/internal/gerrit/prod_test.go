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
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestMakeClient(t *testing.T) {
	t.Parallel()

	Convey("With mocked token server and legacy netrc in datastore", t, func() {
		ctx := memory.Use(context.Background())
		epoch := datastore.RoundTime(testclock.TestRecentTimeUTC)
		ctx, tclock := testclock.UseTime(ctx, testclock.TestRecentTimeUTC)
		ctx = authtest.MockAuthConfig(ctx)

		const gHost = "first.example.com"

		So(datastore.Put(ctx, &netrcToken{gHost, "legacy-1"}), ShouldBeNil)
		f, err := newProd(ctx)
		So(err, ShouldBeNil)

		Convey("factory.token", func() {

			Convey("works", func() {
				Convey("fallback to legacy", func() {
					f.mockMintProjectToken = func(context.Context, auth.ProjectTokenParams) (*auth.Token, error) {
						return nil, nil
					}
					t, err := f.token(ctx, gHost, "not-migrated")
					So(err, ShouldBeNil)
					So(t.AccessToken, ShouldEqual, "bGVnYWN5LTE=") // base64 of "legacy-1"
					So(t.TokenType, ShouldEqual, "Basic")
				})

				Convey("project-scoped account", func() {
					f.mockMintProjectToken = func(context.Context, auth.ProjectTokenParams) (*auth.Token, error) {
						return &auth.Token{Token: "modern-1", Expiry: epoch.Add(2 * time.Minute)}, nil
					}
					t, err := f.token(ctx, gHost, "modern")
					So(err, ShouldBeNil)
					So(t.AccessToken, ShouldEqual, "modern-1")
					So(t.TokenType, ShouldEqual, "Bearer")
				})
			})

			Convey("not works", func() {
				Convey("no legacy host", func() {
					f.mockMintProjectToken = func(context.Context, auth.ProjectTokenParams) (*auth.Token, error) {
						return nil, nil
					}
					_, err := f.token(ctx, "second.example.com", "not-migrated")
					So(err, ShouldErrLike, "No legacy credentials")
				})

				Convey("modern errors out", func() {
					f.mockMintProjectToken = func(context.Context, auth.ProjectTokenParams) (*auth.Token, error) {
						return nil, errors.New("flake")
					}
					_, err := f.token(ctx, gHost, "modern")
					So(err, ShouldErrLike, "flake")
				})
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
			So(datastore.Put(ctx, &netrcToken{u.Host, "legacy-2"}), ShouldBeNil)

			limitedCtx, limitedCancel := context.WithTimeout(ctx, time.Minute)
			defer limitedCancel()

			Convey("project-scoped account", func() {
				tokenCnt := 0
				f.mockMintProjectToken = func(ctx context.Context, _ auth.ProjectTokenParams) (*auth.Token, error) {
					So(ctx.Err(), ShouldBeNil) // must not be expired.
					tokenCnt++
					return &auth.Token{
						Token:  fmt.Sprintf("modern-%d", tokenCnt),
						Expiry: epoch.Add(2 * time.Minute),
					}, nil
				}

				c, err := f.makeClient(limitedCtx, u.Host, "modern")
				So(err, ShouldBeNil)
				_, err = c.ListChanges(limitedCtx, &gerrit.ListChangesRequest{})
				So(err, ShouldBeNil)
				So(requests, ShouldHaveLength, 1)
				So(requests[0].Header["Authorization"], ShouldResemble, []string{"Bearer modern-1"})

				// Ensure clients re-use works even if context expires.
				limitedCancel()
				So(limitedCtx.Err(), ShouldNotBeNil)
				// force token refresh
				tclock.Add(3 * time.Minute)
				c2, err := f.makeClient(ctx, u.Host, "modern")
				So(err, ShouldBeNil)
				So(c2, ShouldEqual, c) // pointer comparison
				_, err = c.ListChanges(ctx, &gerrit.ListChangesRequest{})
				So(err, ShouldBeNil)
				So(requests, ShouldHaveLength, 2)
				So(requests[1].Header["Authorization"], ShouldResemble, []string{"Bearer modern-2"})
			})

			Convey("legacy", func() {
				f.mockMintProjectToken = func(context.Context, auth.ProjectTokenParams) (*auth.Token, error) {
					return nil, nil
				}
				c, err := f.makeClient(limitedCtx, u.Host, "not-migrated")
				So(err, ShouldBeNil)
				_, err = c.ListChanges(limitedCtx, &gerrit.ListChangesRequest{})
				So(err, ShouldBeNil)
				So(requests, ShouldHaveLength, 1)
				tokenB64 := base64.StdEncoding.EncodeToString([]byte("legacy-2"))
				So(requests[0].Header["Authorization"], ShouldResemble, []string{"Basic " + tokenB64})

				// Ensure clients re-use works even if context expires.
				limitedCancel()
				So(limitedCtx.Err(), ShouldNotBeNil)
				c2, err := f.makeClient(ctx, u.Host, "not-migrated")
				So(err, ShouldBeNil)
				So(c2, ShouldEqual, c) // pointer comparison
				_, err = c.ListChanges(ctx, &gerrit.ListChangesRequest{})
				So(err, ShouldBeNil)
				So(requests, ShouldHaveLength, 2)
				So(requests[1].Header["Authorization"], ShouldResemble, []string{"Basic " + tokenB64})
			})
		})
	})
}
