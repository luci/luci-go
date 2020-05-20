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
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"go.chromium.org/luci/server/router"
	"google.golang.org/genproto/googleapis/bytestream"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/resultdb/internal/span"
	"go.chromium.org/luci/resultdb/internal/testutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"

	. "github.com/smartystreets/goconvey/convey"
)

type fakeWriter struct {
	grpc.ClientStream // implements the rest of ByteStream_WriteClient.

	requestsToAccept int
	requests         []*bytestream.WriteRequest
	resErr           error
}

func (*fakeWriter) CloseSend() error {
	return nil
}

func (w *fakeWriter) CloseAndRecv() (*bytestream.WriteResponse, error) {
	if w.resErr != nil {
		return nil, w.resErr
	}
	res := &bytestream.WriteResponse{}
	for _, req := range w.requests {
		res.CommittedSize += int64(len(req.Data))
	}
	return res, nil
}

func (w *fakeWriter) Send(req *bytestream.WriteRequest) error {
	if len(w.requests) < w.requestsToAccept {
		w.requests = append(w.requests, proto.Clone(req).(*bytestream.WriteRequest))
	}
	if len(w.requests) == w.requestsToAccept {
		return io.EOF
	}
	return nil
}

func TestCreateArtifact(t *testing.T) {
	Convey(`CreateArtifact`, t, func() {
		ctx := testutil.SpannerTestContext(t)

		w := &fakeWriter{}
		writerCreated := false
		ach := &artifactCreationHandler{
			RBEInstance: "projects/example/instances/artifacts",
			NewCASWriter: func(context.Context) (bytestream.ByteStream_WriteClient, error) {
				So(writerCreated, ShouldBeFalse)
				writerCreated = true
				return w, nil
			},
		}

		send := func(artifact, hash string, size int64, updateToken, content string) *httptest.ResponseRecorder {
			rec := httptest.NewRecorder()

			// Create a URL whose EscapePath() returns `artifact`.
			u := &url.URL{RawPath: "/" + artifact}
			var err error
			u.Path, err = url.PathUnescape(artifact)
			So(err, ShouldBeNil)

			c := &router.Context{
				Context: ctx,
				Request: &http.Request{
					Header: http.Header{},
					URL:    u,
					Body:   ioutil.NopCloser(strings.NewReader(content)),
				},
				Writer: rec,
			}
			if hash != "" {
				c.Request.Header.Set(artifactContentHashHeaderKey, hash)
			}
			if size != -1 {
				c.Request.Header.Set(artifactContentSizeHeaderKey, strconv.FormatInt(size, 10))
			}
			if updateToken != "" {
				c.Request.Header.Set(UpdateTokenMetadataKey, updateToken)
			}
			ach.Handle(c)
			return rec
		}

		Convey(`Verify request`, func() {
			Convey(`Artifact name is malformed`, func() {
				rec := send("artifact", "", 0, "", "")
				So(rec.Code, ShouldEqual, http.StatusBadRequest)
				So(rec.Body.String(), ShouldContainSubstring, "bad artifact name: does not match")
			})

			Convey(`Hash is missing`, func() {
				rec := send("invocations/inv/artifacts/a", "", 0, "", "")
				So(rec.Code, ShouldEqual, http.StatusBadRequest)
				So(rec.Body.String(), ShouldContainSubstring, "Content-Hash header is missing")
			})

			Convey(`Hash is malformed`, func() {
				rec := send("invocations/inv/artifacts/a", "a", 0, "", "")
				So(rec.Code, ShouldEqual, http.StatusBadRequest)
				So(rec.Body.String(), ShouldContainSubstring, "Content-Hash header value does not match")
			})

			Convey(`Size is missing`, func() {
				rec := send("invocations/inv/artifacts/a", "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", -1, "", "")
				So(rec.Code, ShouldEqual, http.StatusBadRequest)
				So(rec.Body.String(), ShouldContainSubstring, "Content-Length header is missing")
			})

			Convey(`Size is negative`, func() {
				rec := send("invocations/inv/artifacts/a", "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", -100, "", "")
				So(rec.Code, ShouldEqual, http.StatusBadRequest)
				So(rec.Body.String(), ShouldContainSubstring, "Content-Length header must be a value between 0 and 67108864")
			})

			Convey(`Size is too large`, func() {
				rec := send("invocations/inv/artifacts/a", "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", 1<<30, "", "")
				So(rec.Code, ShouldEqual, http.StatusBadRequest)
				So(rec.Body.String(), ShouldContainSubstring, "Content-Length header must be a value between 0 and 67108864")
			})

			Convey(`Update token is missing`, func() {
				rec := send("invocations/inv/artifacts/a", "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", 0, "", "")
				So(rec.Code, ShouldEqual, http.StatusUnauthorized)
				So(rec.Body.String(), ShouldContainSubstring, "Update-Token header is missing")
			})

			Convey(`Update token is invalid`, func() {
				rec := send("invocations/inv/artifacts/a", "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", 0, "x", "")
				So(rec.Code, ShouldEqual, http.StatusForbidden)
				So(rec.Body.String(), ShouldContainSubstring, "invalid Update-Token header value")
			})
		})

		tok, err := invocationTokenKind.Generate(ctx, []byte("inv"), nil, time.Hour)
		So(err, ShouldBeNil)

		Convey(`Verify state`, func() {
			Convey(`Invocation does not exist`, func() {
				rec := send("invocations/inv/artifacts/a", "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", 0, tok, "")
				So(rec.Code, ShouldEqual, http.StatusNotFound)
				So(rec.Body.String(), ShouldContainSubstring, "invocations/inv not found")
			})

			Convey(`Invocation is finalized`, func() {
				testutil.MustApply(ctx, testutil.InsertInvocation("inv", pb.Invocation_FINALIZED, nil))
				rec := send("invocations/inv/artifacts/a", "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", 0, tok, "")
				So(rec.Code, ShouldEqual, http.StatusBadRequest)
				So(rec.Body.String(), ShouldContainSubstring, "invocations/inv is not active")
			})

			Convey(`Same artifact exists`, func() {
				testutil.MustApply(ctx,
					testutil.InsertInvocation("inv", pb.Invocation_ACTIVE, nil),
					span.InsertMap("Artifacts", map[string]interface{}{
						"InvocationId": span.InvocationID("inv"),
						"ParentId":     "",
						"ArtifactId":   "a",
						"RBECASHash":   "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
						"Size":         0,
					}),
				)
				rec := send("invocations/inv/artifacts/a", "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", 0, tok, "")
				So(rec.Code, ShouldEqual, http.StatusNoContent)
			})

			Convey(`Different artifact exists`, func() {
				testutil.MustApply(ctx,
					testutil.InsertInvocation("inv", pb.Invocation_ACTIVE, nil),
					span.InsertMap("Artifacts", map[string]interface{}{
						"InvocationId": span.InvocationID("inv"),
						"ParentId":     "",
						"ArtifactId":   "a",
						"RBECASHash":   "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b851",
						"Size":         1,
					}),
				)
				rec := send("invocations/inv/artifacts/a", "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", 0, tok, "")
				So(rec.Code, ShouldEqual, http.StatusConflict)
			})
		})

		Convey(`Write to CAS`, func() {
			testutil.MustApply(ctx, testutil.InsertInvocation("inv", pb.Invocation_ACTIVE, nil))
			art := "invocations/inv/artifacts/a"

			casSend := func() *httptest.ResponseRecorder {
				return send(art, "sha256:2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824", 5, tok, "hello")
			}

			Convey(`Single request`, func() {
				w.requestsToAccept = 1
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
			})

			Convey(`Invalid digest`, func() {
				w.requestsToAccept = 1
				w.resErr = status.Errorf(codes.InvalidArgument, "wrong hash")
				rec := casSend()
				So(rec.Code, ShouldEqual, http.StatusBadRequest)
				So(rec.Body.String(), ShouldEqual, "Content-Hash and/or Content-Length do not match the request body\n")
			})
		})
	})
}
