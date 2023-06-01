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

	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/server/router"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	pb "go.chromium.org/luci/resultdb/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
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

	Convey(`CreateArtifact`, t, func() {
		ctx := testutil.SpannerTestContext(t)
		ctx, _ = tsmon.WithDummyInMemory(ctx)
		store := tsmon.Store(ctx)

		w := &fakeWriter{}
		writerCreated := false
		ach := &artifactCreationHandler{
			RBEInstance: "projects/example/instances/artifacts",
			NewCASWriter: func(context.Context) (bytestream.ByteStream_WriteClient, error) {
				So(writerCreated, ShouldBeFalse)
				writerCreated = true
				return w, nil
			},
			MaxArtifactContentStreamLength: 1024,
		}

		art := "invocations/inv/artifacts/a"
		tok, err := invocationTokenKind.Generate(ctx, []byte("inv"), nil, time.Hour)
		So(err, ShouldBeNil)

		send := func(artifact, hash string, size int64, updateToken, content, contentType string) *httptest.ResponseRecorder {
			rec := httptest.NewRecorder()

			// Create a URL whose EscapePath() returns `artifact`.
			u := &url.URL{RawPath: "/" + artifact}
			var err error
			u.Path, err = url.PathUnescape(artifact)
			So(err, ShouldBeNil)

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

		Convey(`Verify request`, func() {
			Convey(`Artifact name is malformed`, func() {
				rec := send("artifact", "", 0, "", "", "")
				So(rec.Code, ShouldEqual, http.StatusBadRequest)
				So(rec.Body.String(), ShouldContainSubstring, "bad artifact name: does not match")
			})

			Convey(`Hash is missing`, func() {
				rec := send(art, "", 0, "", "", "")
				So(rec.Code, ShouldEqual, http.StatusBadRequest)
				So(rec.Body.String(), ShouldContainSubstring, "Content-Hash header is missing")
			})

			Convey(`Hash is malformed`, func() {
				rec := send(art, "a", 0, "", "", "")
				So(rec.Code, ShouldEqual, http.StatusBadRequest)
				So(rec.Body.String(), ShouldContainSubstring, "Content-Hash header value does not match")
			})

			Convey(`Size is missing`, func() {
				rec := send(art, "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", -1, "", "", "")
				So(rec.Code, ShouldEqual, http.StatusBadRequest)
				So(rec.Body.String(), ShouldContainSubstring, "Content-Length header is missing")
			})

			Convey(`Size is negative`, func() {
				rec := send(art, "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", -100, "", "", "")
				So(rec.Code, ShouldEqual, http.StatusBadRequest)
				So(rec.Body.String(), ShouldContainSubstring, "Content-Length header must be a value between 0 and 1024")
			})

			Convey(`Size is too large`, func() {
				rec := send(art, "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", 1<<30, "", "", "")
				So(rec.Code, ShouldEqual, http.StatusBadRequest)
				So(rec.Body.String(), ShouldContainSubstring, "Content-Length header must be a value between 0 and 1024")
			})

			Convey(`Update token is missing`, func() {
				rec := send(art, "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", 0, "", "", "")
				So(rec.Code, ShouldEqual, http.StatusUnauthorized)
				So(rec.Body.String(), ShouldContainSubstring, "Update-Token header is missing")
			})

			Convey(`Update token is invalid`, func() {
				rec := send(art, "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", 0, "x", "", "")
				So(rec.Code, ShouldEqual, http.StatusForbidden)
				So(rec.Body.String(), ShouldContainSubstring, "invalid Update-Token header value")
			})

			// RowCount metric should have no changes from any of the above Convey()s.
			So(store.Get(ctx, spanutil.RowCounter, time.Time{}, artMFVs), ShouldBeNil)
		})

		Convey(`Verify state`, func() {
			Convey(`Invocation does not exist`, func() {
				rec := send(art, "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", 0, tok, "", "")
				So(rec.Code, ShouldEqual, http.StatusNotFound)
				So(rec.Body.String(), ShouldContainSubstring, "invocations/inv not found")
			})

			Convey(`Invocation is finalized`, func() {
				testutil.MustApply(ctx, insert.Invocation("inv", pb.Invocation_FINALIZED, nil))
				rec := send(art, "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", 0, tok, "", "")
				So(rec.Code, ShouldEqual, http.StatusBadRequest)
				So(rec.Body.String(), ShouldContainSubstring, "invocations/inv is not active")
			})

			Convey(`Same artifact exists`, func() {
				testutil.MustApply(ctx,
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
				So(rec.Code, ShouldEqual, http.StatusNoContent)
			})

			Convey(`Different artifact exists`, func() {
				testutil.MustApply(ctx,
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
				So(rec.Code, ShouldEqual, http.StatusConflict)
			})

			// RowCount metric should have no changes from any of the above Convey()s.
			So(store.Get(ctx, spanutil.RowCounter, time.Time{}, artMFVs), ShouldBeNil)
		})

		Convey(`Write to CAS`, func() {
			testutil.MustApply(ctx, insert.Invocation("inv", pb.Invocation_ACTIVE, nil))
			w.requestsToAccept = 1

			casSend := func() *httptest.ResponseRecorder {
				return send(art, "sha256:2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824", 5, tok, "hello", "")
			}

			// Skip due to https://crbug.com/1110755
			SkipConvey(`Single request`, func() {
				rec := casSend()
				So(rec.Code, ShouldEqual, http.StatusNoContent)
				So(w.requests, ShouldHaveLength, 1)
				So(w.requests[0].ResourceName, ShouldEqual, "projects/example/instances/artifacts/uploads/0194fdc2-fa2f-fcc0-41d3-ff12045b73c8/blobs/2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824/5")
				So(string(w.requests[0].Data), ShouldEqual, "hello")
			})

			Convey(`Chunked`, func() {
				w.requestsToAccept = 2
				ach.bufSize = 3
				rec := casSend()
				So(rec.Code, ShouldEqual, http.StatusNoContent)
				So(w.requests, ShouldHaveLength, 2)
				So(string(w.requests[0].Data), ShouldEqual, "hel")
				So(string(w.requests[1].Data), ShouldEqual, "lo")
				So(store.Get(ctx, spanutil.RowCounter, time.Time{}, artMFVs), ShouldEqual, 1)
			})

			Convey(`Invalid digest`, func() {
				w.resErr = status.Errorf(codes.InvalidArgument, "wrong hash")
				rec := casSend()
				So(rec.Code, ShouldEqual, http.StatusBadRequest)
				So(rec.Body.String(), ShouldEqual, "Content-Hash and/or Content-Length do not match the request body\n")
				So(store.Get(ctx, spanutil.RowCounter, time.Time{}, artMFVs), ShouldBeNil)
			})

			Convey(`RBE-CAS stops request early`, func() {
				w.requestsToAccept = 1
				w.res = &bytestream.WriteResponse{CommittedSize: 5} // blob already exists
				ach.bufSize = 1
				rec := casSend()
				So(rec.Code, ShouldEqual, http.StatusNoContent)
				// Must not continue requests after receiving io.EOF in the first
				// request.
				So(w.requests, ShouldHaveLength, 1)
				So(store.Get(ctx, spanutil.RowCounter, time.Time{}, artMFVs), ShouldEqual, 1)
			})

		})

		Convey(`Verify digest`, func() {
			testutil.MustApply(ctx, insert.Invocation("inv", pb.Invocation_ACTIVE, nil))
			w.requestsToAccept = 1

			Convey(`Hash`, func() {
				rec := send(art, "sha256:baaaaada5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824", 5, tok, "hello", "")
				So(rec.Code, ShouldEqual, http.StatusBadRequest)
				So(
					rec.Body.String(),
					ShouldEqual,
					`Content-Hash header value "sha256:baaaaada5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824" `+
						`does not match the hash of the request body, "sha256:2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"`+
						"\n")
			})

			Convey(`Size`, func() {
				// Satisfy the RBE-level verification.
				w.res = &bytestream.WriteResponse{CommittedSize: 100}

				rec := send(art, "sha256:2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824", 100, tok, "hello", "")
				So(rec.Code, ShouldEqual, http.StatusBadRequest)
				So(rec.Body.String(), ShouldEqual, "Content-Length header value 100 does not match the length of the request body, 5\n")
			})

			// RowCount metric should have no changes from any of the above Convey()s.
			So(store.Get(ctx, spanutil.RowCounter, time.Time{}, artMFVs), ShouldBeNil)
		})

		Convey(`e2e`, func() {
			testutil.MustApply(ctx, insert.Invocation("inv", pb.Invocation_ACTIVE, nil))

			res := send(art, "sha256:2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824", 5, tok, "hello", "text/plain")
			So(res.Code, ShouldEqual, http.StatusNoContent)

			var actualSize int64
			var actualHash string
			var actualContentType string
			testutil.MustReadRow(ctx, "Artifacts", invocations.ID("inv").Key("", "a"), map[string]any{
				"Size":        &actualSize,
				"RBECASHash":  &actualHash,
				"ContentType": &actualContentType,
			})
			So(actualSize, ShouldEqual, 5)
			So(actualHash, ShouldEqual, "sha256:2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824")
			So(actualContentType, ShouldEqual, "text/plain")
			So(store.Get(ctx, spanutil.RowCounter, time.Time{}, artMFVs), ShouldEqual, 1)
		})
	})
}
