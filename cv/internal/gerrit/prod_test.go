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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
)

func TestMakeClient(t *testing.T) {
	t.Parallel()

	ftt.Run("With mocked token server", t, func(t *ftt.Test) {
		epoch := datastore.RoundTime(testclock.TestRecentTimeUTC)
		ctx, tclock := testclock.UseTime(context.Background(), testclock.TestRecentTimeUTC)
		ctx = authtest.MockAuthConfig(ctx)

		const gHost = "first.example.com"

		f, err := newProd(ctx)
		assert.NoErr(t, err)

		t.Run("factory.token", func(t *ftt.Test) {
			t.Run("works", func(t *ftt.Test) {
				f.mockMintProjectToken = func(context.Context, auth.ProjectTokenParams) (*auth.Token, error) {
					return &auth.Token{Token: "tok-1", Expiry: epoch.Add(2 * time.Minute)}, nil
				}
				tok, err := f.token(ctx, gHost, "lProject")
				assert.NoErr(t, err)
				assert.Loosely(t, tok.AccessToken, should.Equal("tok-1"))
				assert.Loosely(t, tok.TokenType, should.Equal("Bearer"))
			})
			t.Run("return errEmptyProjectToken when minted token is nil", func(t *ftt.Test) {
				f.mockMintProjectToken = func(context.Context, auth.ProjectTokenParams) (*auth.Token, error) {
					return nil, nil
				}
				_, err := f.token(ctx, gHost, "lProject-1")
				assert.Loosely(t, err, should.ErrLike(errEmptyProjectToken))
			})
			t.Run("error", func(t *ftt.Test) {
				f.mockMintProjectToken = func(context.Context, auth.ProjectTokenParams) (*auth.Token, error) {
					return nil, errors.New("flake")
				}
				_, err := f.token(ctx, gHost, "lProject")
				assert.Loosely(t, err, should.ErrLike("flake"))
			})
		})

		t.Run("factory.makeClient", func(t *ftt.Test) {
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
			assert.NoErr(t, err)

			limitedCtx, limitedCancel := context.WithTimeout(ctx, time.Minute)
			defer limitedCancel()

			t.Run("project-scoped account", func(t *ftt.Test) {
				tokenCnt := 0
				f.mockMintProjectToken = func(ctx context.Context, _ auth.ProjectTokenParams) (*auth.Token, error) {
					assert.Loosely(t, ctx.Err(), should.BeNil) // must not be expired.
					tokenCnt++
					return &auth.Token{
						Token:  fmt.Sprintf("tok-%d", tokenCnt),
						Expiry: epoch.Add(2 * time.Minute),
					}, nil
				}

				c, err := f.MakeClient(limitedCtx, u.Host, "lProject")
				assert.NoErr(t, err)
				_, err = c.ListChanges(limitedCtx, &gerritpb.ListChangesRequest{})
				assert.NoErr(t, err)
				assert.Loosely(t, requests, should.HaveLength(1))
				assert.Loosely(t, requests[0].Header["Authorization"], should.Match([]string{"Bearer tok-1"}))

				// Ensure client can be used even if context of its creation expires.
				limitedCancel()
				assert.Loosely(t, limitedCtx.Err(), should.NotBeNil)
				// force token refresh
				tclock.Add(3 * time.Minute)
				_, err = c.ListChanges(ctx, &gerritpb.ListChangesRequest{})
				assert.NoErr(t, err)
				assert.Loosely(t, requests, should.HaveLength(2))
				assert.Loosely(t, requests[1].Header["Authorization"], should.Match([]string{"Bearer tok-2"}))
			})
		})
	})
}
