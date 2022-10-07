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
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"google.golang.org/genproto/googleapis/bytestream"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/secrets"
	"go.chromium.org/luci/server/secrets/testsecrets"

	artifactcontenttest "go.chromium.org/luci/resultdb/internal/artifactcontent/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	pb "go.chromium.org/luci/resultdb/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
)

func TestGenerateSignedURL(t *testing.T) {
	Convey(`TestGenerateSignedURL`, t, func(c C) {
		ctx := testutil.SpannerTestContext(t)

		ctx, _ = testclock.UseTime(ctx, testclock.TestRecentTimeUTC)
		ctx = secrets.Use(ctx, &testsecrets.Store{})
		ctx = authtest.MockAuthConfig(ctx)

		s := &Server{
			HostnameProvider: func(string) string {
				return "results.usercontent.example.com"
			},
		}
		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity: identity.AnonymousIdentity,
		})

		newArt := func(parentID, artID, hash string, gcsUri string) {
			testutil.MustApply(ctx,
				insert.Artifact("inv", parentID, artID, map[string]interface{}{
					"ContentType": "text/plain",
					"Size":        1234,
					"RBECASHash":  hash,
					"GcsURI":      gcsUri,
				}),
			)
		}

		testutil.MustApply(ctx, insert.Invocation("inv", pb.Invocation_FINALIZED, nil))

		Convey(`Basic case`, func() {
			newArt("", "a", "hash", "")
			url, exp, err := s.GenerateSignedURL(ctx, "request.example.com", "invocations/inv/artifacts/a")
			So(err, ShouldBeNil)
			So(url, ShouldStartWith, "https://results.usercontent.example.com/invocations/inv/artifacts/a?token=")
			So(exp, ShouldResemble, clock.Now(ctx).UTC().Add(time.Hour))
		})

		Convey(`Escaped test id`, func() {
			newArt("tr/t/t/r", "a", "hash", "")
			url, exp, err := s.GenerateSignedURL(ctx, "request.example.com", "invocations/inv/tests/t%2Ft/results/r/artifacts/a")
			So(err, ShouldBeNil)
			So(url, ShouldStartWith, "https://results.usercontent.example.com/invocations/inv/tests/t%2Ft/results/r/artifacts/a?token=")
			So(exp, ShouldResemble, clock.Now(ctx).UTC().Add(time.Hour))
		})

		Convey(`Artifact with GcsUri`, func() {
			newArt("", "b", "", "gs://test/file.txt")
			url, exp, err := s.GenerateSignedURL(ctx, "request.example.com", "invocations/inv/artifacts/b")
			So(err, ShouldBeNil)
			So(url, ShouldStartWith, "https://console.developers.google.com/storage/browser/test/file.txt")
			So(exp, ShouldResemble, clock.Now(ctx).UTC().Add(7*24*time.Hour))
		})
	})
}

func TestServeContent(t *testing.T) {
	Convey(`TestServeContent`, t, func(c C) {
		ctx := testutil.SpannerTestContext(t)

		ctx, _ = testclock.UseTime(ctx, testclock.TestRecentTimeUTC)
		ctx = secrets.Use(ctx, &testsecrets.Store{})
		ctx = authtest.MockAuthConfig(ctx)

		casReader := &artifactcontenttest.FakeCASReader{
			Res: []*bytestream.ReadResponse{
				{Data: []byte("contents")},
			},
		}
		var casReadErr error
		s := &Server{
			HostnameProvider: func(string) string {
				return "example.com"
			},
			RBECASInstanceName: "projects/example/instances/artifacts",
			ReadCASBlob: func(ctx context.Context, req *bytestream.ReadRequest) (bytestream.ByteStream_ReadClient, error) {
				casReader.ReadLimit = int(req.ReadLimit)
				return casReader, casReadErr
			},
		}

		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity: identity.AnonymousIdentity,
		})

		fetch := func(rawurl string) (res *http.Response, contents string) {
			req, err := http.NewRequest("GET", rawurl, nil)
			So(err, ShouldBeNil)
			rec := httptest.NewRecorder()
			s.handleGET(&router.Context{
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

		newArt := func(parentID, artID, hash string, datas ...[]byte) {
			casReader.Res = nil
			sum := 0
			for _, d := range datas {
				casReader.Res = append(casReader.Res, &bytestream.ReadResponse{Data: d})
				sum += len(d)
			}
			testutil.MustApply(ctx,
				insert.Artifact("inv", parentID, artID, map[string]interface{}{
					"ContentType": "text/plain",
					"Size":        sum,
					"RBECASHash":  hash,
				}),
			)
		}

		testutil.MustApply(ctx, insert.Invocation("inv", pb.Invocation_FINALIZED, nil))

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
			newArt("tr/t/r", "a", "sha256:deadbeef", []byte("contents"))
			u, _, err := s.GenerateSignedURL(ctx, "request.example.com", "invocations/inv/tests/t/results/r/artifacts/a")
			So(err, ShouldBeNil)
			res, actualContents := fetch(u)
			So(res.StatusCode, ShouldEqual, http.StatusOK)
			So(actualContents, ShouldEqual, "contents")
		})

		Convey(`limit`, func() {
			newArt("tr/t/r", "a", "sha256:deadbeef", []byte("contents"))
			u, _, err := s.GenerateSignedURL(ctx, "request.example.com", "invocations/inv/tests/t/results/r/artifacts/a")
			So(err, ShouldBeNil)

			Convey(`empty`, func() {
				res, body := fetch(u + "&n=")
				So(res.StatusCode, ShouldEqual, http.StatusOK)
				So(body, ShouldEqual, "contents")
				So(res.ContentLength, ShouldEqual, len("contents"))
			})

			Convey(`0`, func() {
				res, body := fetch(u + "&n=0")
				So(res.StatusCode, ShouldEqual, http.StatusOK)
				So(body, ShouldEqual, "contents")
				So(res.ContentLength, ShouldEqual, len("contents"))
			})

			Convey("limit < art_size", func() {
				res, body := fetch(u + "&n=2")
				So(res.StatusCode, ShouldEqual, http.StatusOK)
				So(body, ShouldEqual, "co")
				So(res.ContentLength, ShouldEqual, len("co"))
			})

			Convey("limit > art_size", func() {
				res, body := fetch(u + "&n=100")
				So(res.StatusCode, ShouldEqual, http.StatusOK)
				So(body, ShouldEqual, "contents")
				So(res.ContentLength, ShouldEqual, len("contents"))
			})

			Convey(`multiple`, func() {
				res, body := fetch(u + "&n=4&n=23")
				So(res.StatusCode, ShouldEqual, http.StatusOK)
				So(body, ShouldEqual, "cont")
				So(res.ContentLength, ShouldEqual, len("cont"))
			})

			Convey(`invalid`, func() {
				res, _ := fetch(u + "&n=limit")
				So(res.StatusCode, ShouldEqual, http.StatusBadRequest)
			})
		})

		Convey(`E2E with RBE-CAS`, func() {
			newArt("", "rbe", "sha256:deadbeef", []byte("first "), []byte("second"))
			u, _, err := s.GenerateSignedURL(ctx, "request.example.com", "invocations/inv/artifacts/rbe")
			So(err, ShouldBeNil)

			Convey(`Not found`, func() {
				casReadErr = status.Errorf(codes.NotFound, "not found")
				res, _ := fetch(u)
				So(res.StatusCode, ShouldEqual, http.StatusNotFound)
			})

			Convey(`Not found on first chunk`, func() {
				casReader.ResErr = status.Errorf(codes.NotFound, "not found")
				casReader.ResErrIndex = 0
				res, _ := fetch(u)
				So(res.StatusCode, ShouldEqual, http.StatusNotFound)
			})

			Convey(`Recv error`, func() {
				casReader.ResErr = status.Errorf(codes.Internal, "internal error")
				res, _ := fetch(u)
				So(res.StatusCode, ShouldEqual, http.StatusInternalServerError)
			})

			Convey("Succeeds", func() {
				res, body := fetch(u)
				So(res.StatusCode, ShouldEqual, http.StatusOK)
				So(body, ShouldEqual, "first second")
				So(res.Header.Get("Content-Type"), ShouldEqual, "text/plain")
				So(res.ContentLength, ShouldEqual, len("first second"))
			})
		})
	})
}
