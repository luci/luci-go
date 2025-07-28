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

package recorder

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"google.golang.org/genproto/googleapis/bytestream"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/server/router"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

type fakeWriter struct {
	grpc.ClientStream // implements the rest of ByteStream_WriteClient.

	requestsToAccept int
	requests         []*bytestream.WriteRequest

	res    *bytestream.WriteResponse
	resErr error
}

func (*fakeWriter) CloseSend() error {
	return nil
}

func (w *fakeWriter) CloseAndRecv() (*bytestream.WriteResponse, error) {
	if w.res != nil || w.resErr != nil {
		return w.res, w.resErr
	}

	res := &bytestream.WriteResponse{}
	for _, req := range w.requests {
		res.CommittedSize += int64(len(req.Data))
	}
	return res, nil
}

func (w *fakeWriter) Send(req *bytestream.WriteRequest) error {
	w.requests = append(w.requests, proto.Clone(req).(*bytestream.WriteRequest))
	if len(w.requests) >= w.requestsToAccept {
		return io.EOF
	}
	return nil
}

func TestCreateArtifact(t *testing.T) {
	// metric field values for Artifact table
	artMFVs := []any{string(spanutil.Artifacts), string(spanutil.Inserted), insert.TestRealm}

	ftt.Run(`CreateArtifact`, t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		ctx, _ = tsmon.WithDummyInMemory(ctx)
		store := tsmon.Store(ctx)

		w := &fakeWriter{}
		writerCreated := false
		ach := &artifactCreationHandler{
			RBEInstance: "projects/example/instances/artifacts",
			NewCASWriter: func(context.Context) (bytestream.ByteStream_WriteClient, error) {
				assert.Loosely(t, writerCreated, should.BeFalse)
				writerCreated = true
				return w, nil
			},
			MaxArtifactContentStreamLength: 1024,
		}

		art := "invocations/inv/artifacts/a"
		tok, err := invocationTokenKind.Generate(ctx, []byte("inv"), nil, time.Hour)
		assert.Loosely(t, err, should.BeNil)

		send := func(artifact, hash string, size int64, updateToken, content, contentType string) *httptest.ResponseRecorder {
			rec := httptest.NewRecorder()

			// Create a URL whose EscapePath() returns `artifact`.
			u := &url.URL{RawPath: "/" + artifact}
			var err error
			u.Path, err = url.PathUnescape(artifact)
			assert.Loosely(t, err, should.BeNil)

			c := &router.Context{
				Request: (&http.Request{
					Header: http.Header{},
					URL:    u,
					Body:   io.NopCloser(strings.NewReader(content)),
				}).WithContext(ctx),
				Writer: rec,
			}
			if hash != "" {
				c.Request.Header.Set(artifactContentHashHeaderKey, hash)
			}
			if size != -1 {
				c.Request.Header.Set(artifactContentSizeHeaderKey, strconv.FormatInt(size, 10))
			}
			if updateToken != "" {
				c.Request.Header.Set(updateTokenHeaderKey, updateToken)
			}
			if contentType != "" {
				c.Request.Header.Set(artifactContentTypeHeaderKey, contentType)
			}
			ach.Handle(c)
			return rec
		}

		t.Run(`Verify request`, func(t *ftt.Test) {
			t.Run(`Artifact name is malformed`, func(t *ftt.Test) {
				rec := send("artifact", "", 0, "", "", "")
				assert.Loosely(t, rec.Code, should.Equal(http.StatusBadRequest))
				assert.Loosely(t, rec.Body.String(), should.ContainSubstring("bad artifact name: does not match"))
			})

			t.Run(`Hash is missing`, func(t *ftt.Test) {
				rec := send(art, "", 0, "", "", "")
				assert.Loosely(t, rec.Code, should.Equal(http.StatusBadRequest))
				assert.Loosely(t, rec.Body.String(), should.ContainSubstring("Content-Hash header is missing"))
			})

			t.Run(`Hash is malformed`, func(t *ftt.Test) {
				rec := send(art, "a", 0, "", "", "")
				assert.Loosely(t, rec.Code, should.Equal(http.StatusBadRequest))
				assert.Loosely(t, rec.Body.String(), should.ContainSubstring("Content-Hash header value does not match"))
			})

			t.Run(`Size is missing`, func(t *ftt.Test) {
				rec := send(art, "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", -1, "", "", "")
				assert.Loosely(t, rec.Code, should.Equal(http.StatusBadRequest))
				assert.Loosely(t, rec.Body.String(), should.ContainSubstring("Content-Length header is missing"))
			})

			t.Run(`Size is negative`, func(t *ftt.Test) {
				rec := send(art, "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", -100, "", "", "")
				assert.Loosely(t, rec.Code, should.Equal(http.StatusBadRequest))
				assert.Loosely(t, rec.Body.String(), should.ContainSubstring("Content-Length header must be a value between 0 and 1024"))
			})

			t.Run(`Size is too large`, func(t *ftt.Test) {
				rec := send(art, "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", 1<<30, "", "", "")
				assert.Loosely(t, rec.Code, should.Equal(http.StatusBadRequest))
				assert.Loosely(t, rec.Body.String(), should.ContainSubstring("Content-Length header must be a value between 0 and 1024"))
			})

			t.Run(`Update token is missing`, func(t *ftt.Test) {
				rec := send(art, "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", 0, "", "", "")
				assert.Loosely(t, rec.Code, should.Equal(http.StatusUnauthorized))
				assert.Loosely(t, rec.Body.String(), should.ContainSubstring("Update-Token header is missing"))
			})

			t.Run(`Update token is invalid`, func(t *ftt.Test) {
				rec := send(art, "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", 0, "x", "", "")
				assert.Loosely(t, rec.Code, should.Equal(http.StatusForbidden))
				assert.Loosely(t, rec.Body.String(), should.ContainSubstring("invalid Update-Token header value"))
			})

			// RowCount metric should have no changes from any of the above Convey()s.
			assert.Loosely(t, store.Get(ctx, spanutil.RowCounter, artMFVs), should.BeNil)
		})

		t.Run(`Verify state`, func(t *ftt.Test) {
			t.Run(`Invocation does not exist`, func(t *ftt.Test) {
				rec := send(art, "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", 0, tok, "", "")
				assert.Loosely(t, rec.Code, should.Equal(http.StatusNotFound))
				assert.Loosely(t, rec.Body.String(), should.ContainSubstring("invocations/inv not found"))
			})

			t.Run(`Invocation is finalized`, func(t *ftt.Test) {
				testutil.MustApply(ctx, t, insert.Invocation("inv", pb.Invocation_FINALIZED, nil))
				rec := send(art, "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", 0, tok, "", "")
				assert.Loosely(t, rec.Code, should.Equal(http.StatusBadRequest))
				assert.Loosely(t, rec.Body.String(), should.ContainSubstring("invocations/inv is not active"))
			})

			t.Run(`Same artifact exists`, func(t *ftt.Test) {
				testutil.MustApply(ctx, t,
					insert.Invocation("inv", pb.Invocation_ACTIVE, nil),
					spanutil.InsertMap("Artifacts", map[string]any{
						"InvocationId": invocations.ID("inv"),
						"ParentId":     "",
						"ArtifactId":   "a",
						"RBECASHash":   "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
						"Size":         0,
					}),
				)
				rec := send(art, "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", 0, tok, "", "")
				assert.Loosely(t, rec.Code, should.Equal(http.StatusNoContent))
			})

			t.Run(`Different artifact exists`, func(t *ftt.Test) {
				testutil.MustApply(ctx, t,
					insert.Invocation("inv", pb.Invocation_ACTIVE, nil),
					spanutil.InsertMap("Artifacts", map[string]any{
						"InvocationId": invocations.ID("inv"),
						"ParentId":     "",
						"ArtifactId":   "a",
						"RBECASHash":   "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b851",
						"Size":         1,
					}),
				)
				rec := send(art, "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", 0, tok, "", "")
				assert.Loosely(t, rec.Code, should.Equal(http.StatusConflict))
			})

			// RowCount metric should have no changes from any of the above Convey()s.
			assert.Loosely(t, store.Get(ctx, spanutil.RowCounter, artMFVs), should.BeNil)
		})

		t.Run(`Write to CAS`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t, insert.Invocation("inv", pb.Invocation_ACTIVE, nil))
			w.requestsToAccept = 1

			casSend := func() *httptest.ResponseRecorder {
				return send(art, "sha256:2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824", 5, tok, "hello", "")
			}

			t.Run(`Single request`, func(t *ftt.Test) {
				t.Skip("https://crbug.com/1110755")
				rec := casSend()
				assert.Loosely(t, rec.Code, should.Equal(http.StatusNoContent))
				assert.Loosely(t, w.requests, should.HaveLength(1))
				assert.Loosely(t, w.requests[0].ResourceName, should.Equal("projects/example/instances/artifacts/uploads/0194fdc2-fa2f-fcc0-41d3-ff12045b73c8/blobs/2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824/5"))
				assert.Loosely(t, string(w.requests[0].Data), should.Equal("hello"))
			})

			t.Run(`Chunked`, func(t *ftt.Test) {
				w.requestsToAccept = 2
				ach.bufSize = 3
				rec := casSend()
				assert.Loosely(t, rec.Code, should.Equal(http.StatusNoContent))
				assert.Loosely(t, w.requests, should.HaveLength(2))
				assert.Loosely(t, string(w.requests[0].Data), should.Equal("hel"))
				assert.Loosely(t, string(w.requests[1].Data), should.Equal("lo"))
				assert.Loosely(t, store.Get(ctx, spanutil.RowCounter, artMFVs), should.Equal(1))
			})

			t.Run(`Invalid digest`, func(t *ftt.Test) {
				w.resErr = status.Errorf(codes.InvalidArgument, "wrong hash")
				rec := casSend()
				assert.Loosely(t, rec.Code, should.Equal(http.StatusBadRequest))
				assert.Loosely(t, rec.Body.String(), should.Equal("Content-Hash and/or Content-Length do not match the request body\n"))
				assert.Loosely(t, store.Get(ctx, spanutil.RowCounter, artMFVs), should.BeNil)
			})

			t.Run(`RBE-CAS stops request early`, func(t *ftt.Test) {
				w.requestsToAccept = 1
				w.res = &bytestream.WriteResponse{CommittedSize: 5} // blob already exists
				ach.bufSize = 1
				rec := casSend()
				assert.Loosely(t, rec.Code, should.Equal(http.StatusNoContent))
				// Must not continue requests after receiving io.EOF in the first
				// request.
				assert.Loosely(t, w.requests, should.HaveLength(1))
				assert.Loosely(t, store.Get(ctx, spanutil.RowCounter, artMFVs), should.Equal(1))
			})
		})

		t.Run(`Verify digest`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t, insert.Invocation("inv", pb.Invocation_ACTIVE, nil))
			w.requestsToAccept = 1

			t.Run(`Hash`, func(t *ftt.Test) {
				rec := send(art, "sha256:baaaaada5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824", 5, tok, "hello", "")
				assert.Loosely(t, rec.Code, should.Equal(http.StatusBadRequest))
				assert.Loosely(t,
					rec.Body.String(),
					should.Equal(
						`Content-Hash header value "sha256:baaaaada5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824" `+
							`does not match the hash of the request body, "sha256:2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"`+
							"\n"))
			})

			t.Run(`Size`, func(t *ftt.Test) {
				// Satisfy the RBE-level verification.
				w.res = &bytestream.WriteResponse{CommittedSize: 100}

				rec := send(art, "sha256:2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824", 100, tok, "hello", "")
				assert.Loosely(t, rec.Code, should.Equal(http.StatusBadRequest))
				assert.Loosely(t, rec.Body.String(), should.Equal("Content-Length header value 100 does not match the length of the request body, 5\n"))
			})

			// RowCount metric should have no changes from any of the above Convey()s.
			assert.Loosely(t, store.Get(ctx, spanutil.RowCounter, artMFVs), should.BeNil)
		})

		t.Run(`e2e`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t, insert.Invocation("inv", pb.Invocation_ACTIVE, nil))

			res := send(art, "sha256:2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824", 5, tok, "hello", "text/plain")
			assert.Loosely(t, res.Code, should.Equal(http.StatusNoContent))

			var actualSize int64
			var actualHash string
			var actualContentType string
			testutil.MustReadRow(ctx, t, "Artifacts", invocations.ID("inv").Key("", "a"), map[string]any{
				"Size":        &actualSize,
				"RBECASHash":  &actualHash,
				"ContentType": &actualContentType,
			})
			assert.Loosely(t, actualSize, should.Equal(5))
			assert.Loosely(t, actualHash, should.Equal("sha256:2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"))
			assert.Loosely(t, actualContentType, should.Equal("text/plain"))
			assert.Loosely(t, store.Get(ctx, spanutil.RowCounter, artMFVs), should.Equal(1))
		})
	})
}
