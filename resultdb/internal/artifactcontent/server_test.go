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

package artifactcontent

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/secrets/testsecrets"

	rpcpb "go.chromium.org/luci/resultdb/proto/rpc/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/resultdb/internal/testutil"
)

func TestGenerateSignedURL(t *testing.T) {
	Convey(`TestGenerateSignedURL`, t, func(c C) {
		ctx := TestingContext()

		ctx, _ = testclock.UseTime(ctx, testclock.TestRecentTimeUTC)
		ctx = testsecrets.Use(ctx)
		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity: identity.AnonymousIdentity,
		})
		ctx = authtest.MockAuthConfig(ctx)

		s, err := NewServer(ctx, false, "results.usercontent.example.com")
		So(err, ShouldBeNil)

		Convey(`Basic case`, func() {
			url, exp, err := s.GenerateSignedURL(ctx, "invocations/inv/artifacts/a")
			So(err, ShouldBeNil)
			So(url, ShouldStartWith, "https://results.usercontent.example.com/invocations/inv/artifacts/a?token=")
			So(exp, ShouldResemble, clock.Now(ctx).UTC().Add(time.Hour))
		})

		Convey(`Escaped test id`, func() {
			url, exp, err := s.GenerateSignedURL(ctx, "invocations/inv/tests/t%2Ft/results/r/artifacts/a")
			So(err, ShouldBeNil)
			So(url, ShouldStartWith, "https://results.usercontent.example.com/invocations/inv/tests/t%2Ft/results/r/artifacts/a?token=")
			So(exp, ShouldResemble, clock.Now(ctx).UTC().Add(time.Hour))
		})
	})
}

func TestServeContent(t *testing.T) {
	Convey(`TestServeContent`, t, func(c C) {
		ctx := SpannerTestContext(t)

		ctx, _ = testclock.UseTime(ctx, testclock.TestRecentTimeUTC)

		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity: identity.AnonymousIdentity,
		})
		ctx = authtest.MockAuthConfig(ctx)
		ctx = testsecrets.Use(ctx)

		s, err := NewServer(ctx, false, "example.com")
		So(err, ShouldBeNil)
		s.testFetchIsolate = func(ctx context.Context, isolateURL string, w io.Writer) error {
			return fmt.Errorf("unexpected")
		}

		r := router.NewWithRootContext(ctx)
		s.InstallHandlers(r)

		fetch := func(rawurl string) (res *http.Response, contents string) {
			req, err := http.NewRequest("GET", rawurl, nil)
			So(err, ShouldBeNil)
			rec := httptest.NewRecorder()
			s.handleContent(&router.Context{
				Context: ctx,
				Request: req,
				Writer:  rec,
			})
			res = rec.Result()
			rawContents, err := ioutil.ReadAll(res.Body)
			So(err, ShouldBeNil)
			defer res.Body.Close()
			return res, string(rawContents)
		}

		MustApply(ctx,
			InsertInvocation("inv", rpcpb.Invocation_FINALIZED, nil),
			InsertInvocationArtifact("inv", "a", map[string]interface{}{
				"ContentType": "text/plain",
				"Size":        64,
				"IsolateURL":  "isolate://isolate.example.com/default-gzip/deadbeef",
			}),
			InsertTestResultArtifact("inv", "t/t", "r", "a", map[string]interface{}{
				"ContentType": "text/plain",
				"Size":        64,
				"IsolateURL":  "isolate://isolate.example.com/default-gzip/deadbeef",
			}),
		)

		s.testFetchIsolate = func(ctx context.Context, isolateURL string, w io.Writer) error {
			_, err := w.Write([]byte("contents"))
			return err
		}

		Convey(`Invalid resource name`, func() {
			res, _ := fetch("https://results.usercontent.example.com/invocations/inv")
			So(res.StatusCode, ShouldEqual, http.StatusBadRequest)
		})

		Convey(`Invalid token`, func() {
			res, _ := fetch("https://results.usercontent.example.com/invocations/inv/artifacts/a?token=bad")
			So(res.StatusCode, ShouldEqual, http.StatusForbidden)
		})

		Convey(`No token`, func() {
			res, _ := fetch("https://results.usercontent.example.com/invocations/inv/artifacts/a")
			So(res.StatusCode, ShouldEqual, http.StatusUnauthorized)
		})

		Convey(`Escaped test id`, func() {
			u, _, err := s.GenerateSignedURL(ctx, "invocations/inv/tests/t%2Ft/results/r/artifacts/a")
			So(err, ShouldBeNil)
			res, actualContents := fetch(u)
			So(res.StatusCode, ShouldEqual, http.StatusOK)
			So(actualContents, ShouldEqual, "contents")
		})

		Convey(`E2E`, func() {
			u, _, err := s.GenerateSignedURL(ctx, "invocations/inv/tests/t%2Ft/results/r/artifacts/a")
			So(err, ShouldBeNil)
			res, actualContents := fetch(u)
			So(res.StatusCode, ShouldEqual, http.StatusOK)
			So(actualContents, ShouldEqual, "contents")
			So(res.Header.Get("Content-Type"), ShouldEqual, "text/plain")
			So(res.Header.Get("Content-Length"), ShouldEqual, "64")
		})
	})
}
