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
	"io"
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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/secrets"
	"go.chromium.org/luci/server/secrets/testsecrets"

	artifactcontenttest "go.chromium.org/luci/resultdb/internal/artifactcontent/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestGenerateSignedURL(t *testing.T) {
	ftt.Run(`TestGenerateSignedURL`, t, func(t *ftt.Test) {
		ctx := testutil.TestingContext()

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

		t.Run(`Basic case`, func(t *ftt.Test) {
			url, exp, err := s.GenerateSignedURL(ctx, "request.example.com", "invocations/inv/artifacts/a")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, url, should.HavePrefix("https://results.usercontent.example.com/invocations/inv/artifacts/a?token="))
			assert.Loosely(t, exp, should.Match(clock.Now(ctx).UTC().Add(time.Hour)))
		})

		t.Run(`Escaped test id`, func(t *ftt.Test) {
			url, exp, err := s.GenerateSignedURL(ctx, "request.example.com", "invocations/inv/tests/t%2Ft/results/r/artifacts/a")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, url, should.HavePrefix("https://results.usercontent.example.com/invocations/inv/tests/t%2Ft/results/r/artifacts/a?token="))
			assert.Loosely(t, exp, should.Match(clock.Now(ctx).UTC().Add(time.Hour)))
		})
	})
}

func TestServeContent(t *testing.T) {
	ftt.Run(`TestServeContent`, t, func(t *ftt.Test) {
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

		fetch := func(t testing.TB, rawurl string) (res *http.Response, contents string) {
			req, err := http.NewRequest("GET", rawurl, nil)
			assert.Loosely(t, err, should.BeNil)
			rec := httptest.NewRecorder()
			s.handleGET(&router.Context{
				Request: req.WithContext(ctx),
				Writer:  rec,
			})
			res = rec.Result()
			rawContents, err := io.ReadAll(res.Body)
			assert.Loosely(t, err, should.BeNil)
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
			testutil.MustApply(ctx, t,
				insert.Artifact("inv", parentID, artID, map[string]any{
					"ContentType": "text/plain",
					"Size":        sum,
					"RBECASHash":  hash,
				}),
			)
		}

		testutil.MustApply(ctx, t, insert.Invocation("inv", pb.Invocation_FINALIZED, nil))

		t.Run(`Invalid resource name`, func(t *ftt.Test) {
			res, _ := fetch(t, "https://results.usercontent.example.com/invocations/inv")
			assert.Loosely(t, res.StatusCode, should.Equal(http.StatusBadRequest))
		})

		t.Run(`Invalid token`, func(t *ftt.Test) {
			res, _ := fetch(t, "https://results.usercontent.example.com/invocations/inv/artifacts/a?token=bad")
			assert.Loosely(t, res.StatusCode, should.Equal(http.StatusForbidden))
		})

		t.Run(`No token`, func(t *ftt.Test) {
			res, _ := fetch(t, "https://results.usercontent.example.com/invocations/inv/artifacts/a")
			assert.Loosely(t, res.StatusCode, should.Equal(http.StatusUnauthorized))
		})

		t.Run(`Escaped test id`, func(t *ftt.Test) {
			newArt("tr/t/r", "a", "sha256:deadbeef", []byte("contents"))
			u, _, err := s.GenerateSignedURL(ctx, "request.example.com", "invocations/inv/tests/t/results/r/artifacts/a")
			assert.Loosely(t, err, should.BeNil)
			res, actualContents := fetch(t, u)
			assert.Loosely(t, res.StatusCode, should.Equal(http.StatusOK))
			assert.Loosely(t, actualContents, should.Equal("contents"))
		})

		t.Run(`limit`, func(t *ftt.Test) {
			newArt("tr/t/r", "a", "sha256:deadbeef", []byte("contents"))
			u, _, err := s.GenerateSignedURL(ctx, "request.example.com", "invocations/inv/tests/t/results/r/artifacts/a")
			assert.Loosely(t, err, should.BeNil)

			t.Run(`empty`, func(t *ftt.Test) {
				res, body := fetch(t, u+"&n=")
				assert.Loosely(t, res.StatusCode, should.Equal(http.StatusOK))
				assert.Loosely(t, body, should.Equal("contents"))
				assert.Loosely(t, res.ContentLength, should.Equal(len("contents")))
			})

			t.Run(`0`, func(t *ftt.Test) {
				res, body := fetch(t, u+"&n=0")
				assert.Loosely(t, res.StatusCode, should.Equal(http.StatusOK))
				assert.Loosely(t, body, should.Equal("contents"))
				assert.Loosely(t, res.ContentLength, should.Equal(len("contents")))
			})

			t.Run("limit < art_size", func(t *ftt.Test) {
				res, body := fetch(t, u+"&n=2")
				assert.Loosely(t, res.StatusCode, should.Equal(http.StatusOK))
				assert.Loosely(t, body, should.Equal("co"))
				assert.Loosely(t, res.ContentLength, should.Equal(len("co")))
			})

			t.Run("limit > art_size", func(t *ftt.Test) {
				res, body := fetch(t, u+"&n=100")
				assert.Loosely(t, res.StatusCode, should.Equal(http.StatusOK))
				assert.Loosely(t, body, should.Equal("contents"))
				assert.Loosely(t, res.ContentLength, should.Equal(len("contents")))
			})

			t.Run(`multiple`, func(t *ftt.Test) {
				res, body := fetch(t, u+"&n=4&n=23")
				assert.Loosely(t, res.StatusCode, should.Equal(http.StatusOK))
				assert.Loosely(t, body, should.Equal("cont"))
				assert.Loosely(t, res.ContentLength, should.Equal(len("cont")))
			})

			t.Run(`invalid`, func(t *ftt.Test) {
				res, _ := fetch(t, u+"&n=limit")
				assert.Loosely(t, res.StatusCode, should.Equal(http.StatusBadRequest))
			})
		})

		t.Run(`E2E with RBE-CAS`, func(t *ftt.Test) {
			newArt("", "rbe", "sha256:deadbeef", []byte("first "), []byte("second"))
			u, _, err := s.GenerateSignedURL(ctx, "request.example.com", "invocations/inv/artifacts/rbe")
			assert.Loosely(t, err, should.BeNil)

			t.Run(`Not found`, func(t *ftt.Test) {
				casReadErr = status.Errorf(codes.NotFound, "not found")
				res, _ := fetch(t, u)
				assert.Loosely(t, res.StatusCode, should.Equal(http.StatusNotFound))
			})

			t.Run(`Not found on first chunk`, func(t *ftt.Test) {
				casReader.ResErr = status.Errorf(codes.NotFound, "not found")
				casReader.ResErrIndex = 0
				res, _ := fetch(t, u)
				assert.Loosely(t, res.StatusCode, should.Equal(http.StatusNotFound))
			})

			t.Run(`Recv error`, func(t *ftt.Test) {
				casReader.ResErr = status.Errorf(codes.Internal, "internal error")
				res, _ := fetch(t, u)
				assert.Loosely(t, res.StatusCode, should.Equal(http.StatusInternalServerError))
			})

			t.Run("Succeeds", func(t *ftt.Test) {
				res, body := fetch(t, u)
				assert.Loosely(t, res.StatusCode, should.Equal(http.StatusOK))
				assert.Loosely(t, body, should.Equal("first second"))
				assert.Loosely(t, res.Header.Get("Content-Type"), should.Equal("text/plain"))
				assert.Loosely(t, res.ContentLength, should.Equal(len("first second")))
			})
		})
	})
}
