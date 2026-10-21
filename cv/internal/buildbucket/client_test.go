// Copyright 2026 The LUCI Authors.
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

package buildbucket

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"
	"google.golang.org/grpc/codes"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/caching"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestMakeClient(t *testing.T) {
	t.Parallel()

	ftt.Run("MakeClient disables fallback to CV service account", t, func(t *ftt.Test) {
		ctx, tc := testclock.UseTime(context.Background(), testclock.TestRecentTimeUTC)
		tc.SetTimerCallback(func(d time.Duration, t clock.Timer) {
			if !testclock.HasTags(t, clock.ContextDeadlineTag) {
				tc.Add(d)
			}
		})
		ctx = caching.WithEmptyProcessCache(ctx)

		const (
			tokenServerHost = "tokens.example.com"
			targetHost      = "external-bb.example.com"
			luciProject     = "test-proj"
		)

		var targetReqs []*http.Request
		ctx = auth.ModifyConfig(ctx, func(cfg auth.Config) auth.Config {
			cfg.AccessTokenProvider = func(_ context.Context, scopes []string) (*oauth2.Token, error) {
				return &oauth2.Token{
					AccessToken: "cv-service-account-token:" + strings.Join(scopes, ","),
					TokenType:   "Bearer",
					Expiry:      testclock.TestRecentTimeUTC.Add(time.Hour),
				}, nil
			}
			cfg.AnonymousTransport = func(context.Context) http.RoundTripper {
				return roundTripperFunc(func(req *http.Request) (*http.Response, error) {
					if req.URL.Host == tokenServerHost && req.URL.Path == "/prpc/tokenserver.minter.TokenMinter/MintProjectToken" {
						// Simulate a LUCI project without a project-scoped service account.
						hdr := make(http.Header)
						hdr.Set("X-Prpc-Grpc-Code", strconv.Itoa(int(codes.NotFound)))
						hdr.Set("Content-Type", "text/plain")
						return &http.Response{
							StatusCode: http.StatusNotFound,
							Header:     hdr,
							Body:       io.NopCloser(strings.NewReader("project identity not found")),
							Request:    req,
						}, nil
					}
					targetReqs = append(targetReqs, req)
					hdr := make(http.Header)
					hdr.Set("X-Prpc-Grpc-Code", strconv.Itoa(int(codes.OK)))
					hdr.Set("Content-Type", "application/prpc; encoding=binary")
					return &http.Response{
						StatusCode: http.StatusOK,
						Header:     hdr,
						Body:       io.NopCloser(strings.NewReader("")),
						Request:    req,
					}, nil
				})
			}
			return cfg
		})

		ctx = auth.WithState(ctx, &authtest.FakeState{
			FakeDB: authtest.NewFakeDB(
				authtest.MockTokenServiceURL("https://" + tokenServerHost),
			),
		})

		client, err := NewClientFactory().MakeClient(ctx, targetHost, luciProject)
		assert.NoErr(t, err)

		_, err = client.SearchBuilds(ctx, &bbpb.SearchBuildsRequest{})
		assert.ErrIsLike(t, err, `project "test-proj" doesn't have a project-scoped account, can't act as this project`)
		assert.Loosely(t, targetReqs, should.BeEmpty)
	})
}
