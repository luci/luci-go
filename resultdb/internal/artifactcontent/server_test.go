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
	neturl "net/url"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
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
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/internal/workunits"
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
			url, exp, err := s.GenerateSignedURL(ctx, "request.example.com", "rootInvocations/inv/workUnits/wu/artifacts/a", nil)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, url, should.HavePrefix("https://results.usercontent.example.com/rootInvocations/inv/workUnits/wu/artifacts/a?token="))
			assert.Loosely(t, exp, should.Match(clock.Now(ctx).UTC().Add(time.Hour)))
		})

		t.Run(`Escaped test id`, func(t *ftt.Test) {
			url, exp, err := s.GenerateSignedURL(ctx, "request.example.com", "rootInvocations/inv/workUnits/wu/tests/t%2Ft/results/r/artifacts/a", nil)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, url, should.HavePrefix("https://results.usercontent.example.com/rootInvocations/inv/workUnits/wu/tests/t%2Ft/results/r/artifacts/a?token="))
			assert.Loosely(t, exp, should.Match(clock.Now(ctx).UTC().Add(time.Hour)))
		})

		t.Run(`With project`, func(t *ftt.Test) {
			url, _, err := s.GenerateSignedURL(ctx, "request.example.com", "rootInvocations/inv/workUnits/wu/artifacts/a", map[string]string{"project": "test-project"})
			assert.Loosely(t, err, should.BeNil)

			// Validate the token.
			parsedURL, err := neturl.Parse(url)
			assert.Loosely(t, err, should.BeNil)
			token := parsedURL.Query().Get("token")
			assert.Loosely(t, token, should.NotBeEmpty)

			embedded, err := artifactNameTokenKind.Validate(ctx, token, []byte("rootInvocations/inv/workUnits/wu/artifacts/a"))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, embedded, should.Resemble(map[string]string{"project": "test-project"}))
		})

		t.Run(`With legacy invocation`, func(t *ftt.Test) {
			url, exp, err := s.GenerateSignedURL(ctx, "request.example.com", "invocations/inv/tests/t%2Ft/results/r/artifacts/a", nil)
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
				casReader.ReadOffset = int(req.ReadOffset)
				casReader.ReadLimit = int(req.ReadLimit)
				return casReader, casReadErr
			},
		}

		var casReadByProjectErr error
		var projectFromReader string
		s.ReadCASBlobByProject = func(ctx context.Context, req *bytestream.ReadRequest, project string) (bytestream.ByteStream_ReadClient, error) {
			casReader.ReadOffset = int(req.ReadOffset)
			casReader.ReadLimit = int(req.ReadLimit)
			projectFromReader = project
			return casReader, casReadByProjectErr
		}

		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity: identity.AnonymousIdentity,
		})

		fetchWithHeaders := func(t testing.TB, rawurl string, headers map[string]string) (res *http.Response, contents string) {
			req, err := http.NewRequest("GET", rawurl, nil)
			assert.Loosely(t, err, should.BeNil)
			for k, v := range headers {
				req.Header.Set(k, v)
			}
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

		fetch := func(t testing.TB, rawurl string) (res *http.Response, contents string) {
			return fetchWithHeaders(t, rawurl, nil)
		}

		newArt := func(parentID, artID, hash string, datas ...[]byte) {
			casReader.Res = nil
			sum := 0
			for _, d := range datas {
				casReader.Res = append(casReader.Res, &bytestream.ReadResponse{Data: d})
				sum += len(d)
			}
			wuID := workunits.ID{RootInvocationID: "inv", WorkUnitID: "wu"}
			testutil.MustApply(ctx, t,
				insert.Artifact(wuID.LegacyInvocationID(), parentID, artID, map[string]any{
					"ContentType": "text/plain",
					"Size":        sum,
					"RBECASHash":  hash,
				}),
			)
		}

		newLegacyArt := func(inv invocations.ID, parentID, artID, hash string, datas ...[]byte) {
			casReader.Res = nil
			sum := 0
			for _, d := range datas {
				casReader.Res = append(casReader.Res, &bytestream.ReadResponse{Data: d})
				sum += len(d)
			}
			testutil.MustApply(ctx, t,
				insert.Artifact(inv, parentID, artID, map[string]any{
					"ContentType": "text/plain",
					"Size":        sum,
					"RBECASHash":  hash,
				}),
			)
		}

		newArtWithURI := func(parentID, artID, uri string, datas ...[]byte) {
			casReader.Res = nil
			sum := 0
			for _, d := range datas {
				casReader.Res = append(casReader.Res, &bytestream.ReadResponse{Data: d})
				sum += len(d)
			}
			wuID := workunits.ID{RootInvocationID: "inv", WorkUnitID: "wu"}
			testutil.MustApply(ctx, t,
				insert.Artifact(wuID.LegacyInvocationID(), parentID, artID, map[string]any{
					"ContentType": "text/plain",
					"Size":        sum,
					"RbeURI":      uri,
				}),
			)
		}

		// Create some work units and invocations to create artifacts in.
		ms := []*spanner.Mutation{}
		ms = append(ms, insert.RootInvocationWithRootWorkUnit(rootinvocations.NewBuilder("inv").Build())...)
		ms = append(ms, insert.WorkUnit(workunits.NewBuilder("inv", "wu").Build())...)
		ms = append(ms, insert.Invocation("legacyinv", pb.Invocation_FINALIZED, nil))
		testutil.MustApply(ctx, t, ms...)

		t.Run(`Invalid resource name`, func(t *ftt.Test) {
			t.Run(`Root invocation-like`, func(t *ftt.Test) {
				res, _ := fetch(t, "https://results.usercontent.example.com/rootInvocations/inv")
				assert.Loosely(t, res.StatusCode, should.Equal(http.StatusBadRequest))
			})
			t.Run(`Legacy-style`, func(t *ftt.Test) {
				res, _ := fetch(t, "https://results.usercontent.example.com/invocations/inv")
				assert.Loosely(t, res.StatusCode, should.Equal(http.StatusBadRequest))
			})
		})

		t.Run(`Invalid token`, func(t *ftt.Test) {
			res, _ := fetch(t, "https://results.usercontent.example.com/rootInvocations/inv/workUnits/wu/artifacts/a?token=bad")
			assert.Loosely(t, res.StatusCode, should.Equal(http.StatusForbidden))
		})

		t.Run(`No token`, func(t *ftt.Test) {
			res, _ := fetch(t, "https://results.usercontent.example.com/rootInvocations/inv/workUnits/wu/artifacts/a")
			assert.Loosely(t, res.StatusCode, should.Equal(http.StatusUnauthorized))
		})

		t.Run(`Escaped test id`, func(t *ftt.Test) {
			newArt("tr/t/r", "a", "sha256:deadbeef", []byte("contents"))
			u, _, err := s.GenerateSignedURL(ctx, "request.example.com", "rootInvocations/inv/workUnits/wu/tests/t/results/r/artifacts/a", nil)
			assert.Loosely(t, err, should.BeNil)
			res, actualContents := fetch(t, u)
			assert.Loosely(t, res.StatusCode, should.Equal(http.StatusOK))
			assert.Loosely(t, actualContents, should.Equal("contents"))
		})

		t.Run(`limit`, func(t *ftt.Test) {
			newArt("tr/t/r", "a", "sha256:deadbeef", []byte("contents"))
			u, _, err := s.GenerateSignedURL(ctx, "request.example.com", "rootInvocations/inv/workUnits/wu/tests/t/results/r/artifacts/a", nil)
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

		t.Run(`offset`, func(t *ftt.Test) {
			newArt("tr/t/r", "a", "sha256:deadbeef", []byte("contents"))
			u, _, err := s.GenerateSignedURL(ctx, "request.example.com", "rootInvocations/inv/workUnits/wu/tests/t/results/r/artifacts/a", nil)
			assert.Loosely(t, err, should.BeNil)

			t.Run(`empty`, func(t *ftt.Test) {
				res, body := fetch(t, u+"&offset=")
				assert.Loosely(t, res.StatusCode, should.Equal(http.StatusOK))
				assert.Loosely(t, body, should.Equal("contents"))
				assert.Loosely(t, res.ContentLength, should.Equal(len("contents")))
			})

			t.Run(`0`, func(t *ftt.Test) {
				res, body := fetch(t, u+"&offset=0")
				assert.Loosely(t, res.StatusCode, should.Equal(http.StatusOK))
				assert.Loosely(t, body, should.Equal("contents"))
				assert.Loosely(t, res.ContentLength, should.Equal(len("contents")))
			})

			t.Run("offset < art_size", func(t *ftt.Test) {
				res, body := fetch(t, u+"&offset=2")
				assert.Loosely(t, res.StatusCode, should.Equal(http.StatusOK))
				assert.Loosely(t, body, should.Equal("ntents"))
				assert.Loosely(t, res.ContentLength, should.Equal(len("ntents")))
			})

			t.Run("offset and limit < art_size", func(t *ftt.Test) {
				res, body := fetch(t, u+"&offset=2&n=3")
				assert.Loosely(t, res.StatusCode, should.Equal(http.StatusOK))
				assert.Loosely(t, body, should.Equal("nte"))
				assert.Loosely(t, res.ContentLength, should.Equal(len("nte")))
			})

			t.Run("offset and limit > art_size", func(t *ftt.Test) {
				res, body := fetch(t, u+"&offset=6&n=10")
				assert.Loosely(t, res.StatusCode, should.Equal(http.StatusOK))
				assert.Loosely(t, body, should.Equal("ts"))
				assert.Loosely(t, res.ContentLength, should.Equal(len("ts")))
			})

			t.Run("offset >= art_size", func(t *ftt.Test) {
				res, body := fetch(t, u+"&offset=8&n=5")
				assert.Loosely(t, res.StatusCode, should.Equal(http.StatusOK))
				assert.Loosely(t, body, should.BeEmpty)
				assert.Loosely(t, res.ContentLength, should.Equal(0))
			})

			t.Run(`negative`, func(t *ftt.Test) {
				res, _ := fetch(t, u+"&offset=-1")
				assert.Loosely(t, res.StatusCode, should.Equal(http.StatusBadRequest))
			})

			t.Run(`invalid`, func(t *ftt.Test) {
				res, _ := fetch(t, u+"&offset=abc")
				assert.Loosely(t, res.StatusCode, should.Equal(http.StatusBadRequest))
			})
		})

		t.Run(`Range header`, func(t *ftt.Test) {
			newArt("tr/t/r", "a", "sha256:deadbeef", []byte("contents"))
			u, _, err := s.GenerateSignedURL(ctx, "request.example.com", "rootInvocations/inv/workUnits/wu/tests/t/results/r/artifacts/a", nil)
			assert.Loosely(t, err, should.BeNil)

			t.Run(`start and end`, func(t *ftt.Test) {
				res, body := fetchWithHeaders(t, u, map[string]string{"Range": "bytes=2-4"})
				assert.Loosely(t, res.StatusCode, should.Equal(http.StatusPartialContent))
				assert.Loosely(t, body, should.Equal("nte"))
				assert.Loosely(t, res.ContentLength, should.Equal(3))
				assert.Loosely(t, res.Header.Get("Content-Range"), should.Equal("bytes 2-4/8"))
				assert.Loosely(t, res.Header.Get("Accept-Ranges"), should.Equal("bytes"))
			})

			t.Run(`open-ended`, func(t *ftt.Test) {
				res, body := fetchWithHeaders(t, u, map[string]string{"Range": "bytes=2-"})
				assert.Loosely(t, res.StatusCode, should.Equal(http.StatusPartialContent))
				assert.Loosely(t, body, should.Equal("ntents"))
				assert.Loosely(t, res.ContentLength, should.Equal(6))
				assert.Loosely(t, res.Header.Get("Content-Range"), should.Equal("bytes 2-7/8"))
			})

			t.Run(`end >= art_size`, func(t *ftt.Test) {
				res, body := fetchWithHeaders(t, u, map[string]string{"Range": "bytes=2-100"})
				assert.Loosely(t, res.StatusCode, should.Equal(http.StatusPartialContent))
				assert.Loosely(t, body, should.Equal("ntents"))
				assert.Loosely(t, res.ContentLength, should.Equal(6))
				assert.Loosely(t, res.Header.Get("Content-Range"), should.Equal("bytes 2-7/8"))
			})

			t.Run(`suffix range`, func(t *ftt.Test) {
				res, body := fetchWithHeaders(t, u, map[string]string{"Range": "bytes=-3"})
				assert.Loosely(t, res.StatusCode, should.Equal(http.StatusPartialContent))
				assert.Loosely(t, body, should.Equal("nts"))
				assert.Loosely(t, res.ContentLength, should.Equal(3))
				assert.Loosely(t, res.Header.Get("Content-Range"), should.Equal("bytes 5-7/8"))
			})

			t.Run(`suffix range >= art_size`, func(t *ftt.Test) {
				res, body := fetchWithHeaders(t, u, map[string]string{"Range": "bytes=-100"})
				assert.Loosely(t, res.StatusCode, should.Equal(http.StatusPartialContent))
				assert.Loosely(t, body, should.Equal("contents"))
				assert.Loosely(t, res.ContentLength, should.Equal(8))
				assert.Loosely(t, res.Header.Get("Content-Range"), should.Equal("bytes 0-7/8"))
			})

			t.Run(`unsatisfiable start >= art_size`, func(t *ftt.Test) {
				res, _ := fetchWithHeaders(t, u, map[string]string{"Range": "bytes=8-10"})
				assert.Loosely(t, res.StatusCode, should.Equal(http.StatusRequestedRangeNotSatisfiable))
				assert.Loosely(t, res.Header.Get("Content-Range"), should.Equal("bytes */8"))
			})

			t.Run(`unsatisfiable suffix 0`, func(t *ftt.Test) {
				res, _ := fetchWithHeaders(t, u, map[string]string{"Range": "bytes=-0"})
				assert.Loosely(t, res.StatusCode, should.Equal(http.StatusRequestedRangeNotSatisfiable))
				assert.Loosely(t, res.Header.Get("Content-Range"), should.Equal("bytes */8"))
			})

			t.Run(`invalid unit`, func(t *ftt.Test) {
				res, _ := fetchWithHeaders(t, u, map[string]string{"Range": "items=0-5"})
				assert.Loosely(t, res.StatusCode, should.Equal(http.StatusBadRequest))
			})

			t.Run(`invalid end < start`, func(t *ftt.Test) {
				res, _ := fetchWithHeaders(t, u, map[string]string{"Range": "bytes=5-2"})
				assert.Loosely(t, res.StatusCode, should.Equal(http.StatusBadRequest))
			})

			t.Run(`multiple ranges unsupported`, func(t *ftt.Test) {
				res, _ := fetchWithHeaders(t, u, map[string]string{"Range": "bytes=0-1, 3-4"})
				assert.Loosely(t, res.StatusCode, should.Equal(http.StatusBadRequest))
			})

			t.Run(`combined with query params rejected`, func(t *ftt.Test) {
				res, _ := fetchWithHeaders(t, u+"&n=5", map[string]string{"Range": "bytes=0-4"})
				assert.Loosely(t, res.StatusCode, should.Equal(http.StatusBadRequest))

				res, _ = fetchWithHeaders(t, u+"&offset=2", map[string]string{"Range": "bytes=2-4"})
				assert.Loosely(t, res.StatusCode, should.Equal(http.StatusBadRequest))
			})
		})

		t.Run(`E2E with RBE-CAS`, func(t *ftt.Test) {
			newArt("", "rbe", "sha256:deadbeef", []byte("first "), []byte("second"))
			u, _, err := s.GenerateSignedURL(ctx, "request.example.com", "rootInvocations/inv/workUnits/wu/artifacts/rbe", nil)
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

			t.Run("Succeeds with byte range across chunks", func(t *ftt.Test) {
				res, body := fetchWithHeaders(t, u, map[string]string{"Range": "bytes=4-8"})
				assert.Loosely(t, res.StatusCode, should.Equal(http.StatusPartialContent))
				assert.Loosely(t, body, should.Equal("t sec"))
				assert.Loosely(t, res.Header.Get("Content-Range"), should.Equal("bytes 4-8/12"))
				assert.Loosely(t, res.ContentLength, should.Equal(5))
			})
		})

		t.Run(`E2E with RBE-CAS URI`, func(t *ftt.Test) {
			newArtWithURI("", "rbe-uri", "bytestream://remotebuildexecution.googleapis.com/projects/example/instances/artifacts/blobs/deadbeef/123", []byte("first "), []byte("second"))
			u, _, err := s.GenerateSignedURL(ctx, "request.example.com", "rootInvocations/inv/workUnits/wu/artifacts/rbe-uri", map[string]string{"project": "test-project"})
			assert.Loosely(t, err, should.BeNil)

			t.Run("Succeeds", func(t *ftt.Test) {
				res, body := fetch(t, u)
				assert.Loosely(t, res.StatusCode, should.Equal(http.StatusOK))
				assert.Loosely(t, body, should.Equal("first second"))
				assert.Loosely(t, res.Header.Get("Content-Type"), should.Equal("text/plain"))
				assert.Loosely(t, res.ContentLength, should.Equal(len("first second")))
				assert.Loosely(t, projectFromReader, should.Equal("test-project"))
			})

			t.Run("Succeeds with offset and limit", func(t *ftt.Test) {
				res, body := fetch(t, u+"&offset=4&n=5")
				assert.Loosely(t, res.StatusCode, should.Equal(http.StatusOK))
				assert.Loosely(t, body, should.Equal("t sec"))
				assert.Loosely(t, res.ContentLength, should.Equal(5))
				assert.Loosely(t, projectFromReader, should.Equal("test-project"))
			})

			t.Run(`No project in token`, func(t *ftt.Test) {
				u, _, err := s.GenerateSignedURL(ctx, "request.example.com", "rootInvocations/inv/workUnits/wu/artifacts/rbe-uri", nil)
				assert.Loosely(t, err, should.BeNil)
				res, _ := fetch(t, u)
				assert.Loosely(t, res.StatusCode, should.Equal(http.StatusForbidden))
			})
		})

		t.Run(`with legacy invocation ID`, func(t *ftt.Test) {
			newLegacyArt("legacyinv", "tr/t/r", "a", "sha256:deadbeef", []byte("contents"))
			u, _, err := s.GenerateSignedURL(ctx, "request.example.com", "invocations/legacyinv/tests/t/results/r/artifacts/a", nil)
			assert.Loosely(t, err, should.BeNil)
			res, actualContents := fetch(t, u)
			assert.Loosely(t, res.StatusCode, should.Equal(http.StatusOK))
			assert.Loosely(t, actualContents, should.Equal("contents"))
		})
	})
}
