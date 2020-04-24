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

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/secrets/testsecrets"

	. "go.chromium.org/luci/resultdb/internal/testutil"
	rpcpb "go.chromium.org/luci/resultdb/proto/rpc/v1"

	. "github.com/smartystreets/goconvey/convey"
)

func TestArtifactContent(t *testing.T) {
	Convey(`TestArtifactContent`, t, func(c C) {
		ctx := SpannerTestContext(t)

		ctx, _ = testclock.UseTime(ctx, testclock.TestRecentTimeUTC)

		ctx = authtest.MockAuthConfig(ctx)
		authState := &authtest.FakeState{}
		ctx = auth.WithState(ctx, authState)
		ctx = testsecrets.Use(ctx)

		s, err := NewServer(ctx, false, "results.usercontent.example.com")
		s.testFetchIsolate = func(ctx context.Context, isolateHost, ns, digest string, w io.Writer) error {
			return fmt.Errorf("unexpected")
		}
		So(err, ShouldBeNil)

		r := router.NewWithRootContext(ctx)
		s.InstallHandlers(r)
		s.testFetchIsolate = func(ctx context.Context, isolateHost, ns, digest string, w io.Writer) error {
			_, err := w.Write([]byte("contents"))
			return err
		}

		fetch := func(url string) (res *http.Response, contents string) {
			// Then request the content.
			contentReq, err := http.NewRequest("GET", url, nil)
			So(err, ShouldBeNil)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, contentReq)
			res = rec.Result()
			rawContents, err := ioutil.ReadAll(res.Body)
			So(err, ShouldBeNil)
			return res, string(rawContents)
		}

		MustApply(ctx,
			InsertInvocation("inv", rpcpb.Invocation_FINALIZED, nil),
			InsertInvocationArtifact("inv", "a", map[string]interface{}{
				"ContentType": "text/plain",
				"Size":        64,
				"IsolateURL":  "isolate://isolate.example.com/default-gzip/deadbeef",
			}),
			InsertTestResultArtifact("inv", "t t", "r", "a", map[string]interface{}{
				"ContentType": "text/plain",
				"Size":        64,
				"IsolateURL":  "isolate://isolate.example.com/default-gzip/deadbeef",
			}),
		)

		Convey(`URL sanity check`, func() {
			// Validate generated signed URL.
			u, _, err := s.GenerateSignedURL(ctx, "invocations/inv/artifacts/a")
			So(err, ShouldBeNil)

			So(u.Query().Get("token"), ShouldNotEqual, "")
			u.RawQuery = ""
			So(u.String(), ShouldEqual, "https://results.usercontent.example.com/invocations/inv/artifacts/a")
		})

		Convey(`No token`, func() {
			res, _ := fetch("https://results.usercontent.example.com/invocations/inv/artifacts/a")
			So(res.StatusCode, ShouldEqual, http.StatusUnauthorized)
		})

		Convey(`Invalid token`, func() {
			res, _ := fetch("https://results.usercontent.example.com/invocations/inv/artifacts/a?token=bad")
			So(res.StatusCode, ShouldEqual, http.StatusForbidden)
		})

		Convey(`Escaped test id`, func() {
			u, exp, err := s.GenerateSignedURL(ctx, "invocations/inv/tests/t%20/t/results/r/artifacts/a")
			So(err, ShouldBeNil)
			So(exp, ShouldResemble, clock.Now(ctx).UTC().Add(time.Hour))

			res, actualContents := fetch(u.String())
			So(res.StatusCode, ShouldEqual, http.StatusOK)
			So(actualContents, ShouldEqual, "contents")
			So(res.Header.Get("Content-Type"), ShouldEqual, "text/plain")
			So(res.Header.Get("Content-Length"), ShouldEqual, "64")
		})

		Convey(`E2E`, func() {
			s.testFetchIsolate = func(ctx context.Context, isolateHost, ns, digest string, w io.Writer) error {
				_, err := w.Write([]byte("contents"))
				return err
			}
			u, exp, err := s.GenerateSignedURL(ctx, "invocations/inv/artifacts/a")
			So(err, ShouldBeNil)
			So(exp, ShouldResemble, clock.Now(ctx).UTC().Add(time.Hour))

			res, actualContents := fetch(u.String())
			So(res.StatusCode, ShouldEqual, http.StatusOK)
			So(actualContents, ShouldEqual, "contents")
			So(res.Header.Get("Content-Type"), ShouldEqual, "text/plain")
			So(res.Header.Get("Content-Length"), ShouldEqual, "64")
		})
	})
}
