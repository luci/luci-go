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
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/julienschmidt/httprouter"

	"go.chromium.org/luci/server/router"

	"go.chromium.org/luci/resultdb/internal/span"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/resultdb/internal/testutil"
)

func TestCreateArtifact(t *testing.T) {
	Convey(`CreateArtifact`, t, func() {
		ctx := SpannerTestContext(t)

		send := func(artifact, hash string, size int64, updateToken string) *httptest.ResponseRecorder {
			rec := httptest.NewRecorder()
			c := &router.Context{
				Context: ctx,
				Params:  httprouter.Params{{Key: "artifact", Value: artifact}},
				Request: &http.Request{Header: http.Header{}},
				Writer:  rec,
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
			handleArtifactCreation(c)
			return rec
		}

		Convey(`Verify request`, func() {
			Convey(`Artifact name is malformed`, func() {
				rec := send("artifact", "", 0, "")
				So(rec.Code, ShouldEqual, http.StatusBadRequest)
				So(rec.Body.String(), ShouldContainSubstring, "bad artifact name: does not match")
			})

			Convey(`Hash is missing`, func() {
				rec := send("invocations/inv/artifacts/a", "", 0, "")
				So(rec.Code, ShouldEqual, http.StatusBadRequest)
				So(rec.Body.String(), ShouldContainSubstring, "Content-Hash header is missing")
			})

			Convey(`Hash is malformed`, func() {
				rec := send("invocations/inv/artifacts/a", "a", 0, "")
				So(rec.Code, ShouldEqual, http.StatusBadRequest)
				So(rec.Body.String(), ShouldContainSubstring, "Content-Hash header value does not match")
			})

			Convey(`Size is missing`, func() {
				rec := send("invocations/inv/artifacts/a", "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", -1, "")
				So(rec.Code, ShouldEqual, http.StatusBadRequest)
				So(rec.Body.String(), ShouldContainSubstring, "Content-Length header is missing")
			})

			Convey(`Size is negative`, func() {
				rec := send("invocations/inv/artifacts/a", "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", -100, "")
				So(rec.Code, ShouldEqual, http.StatusBadRequest)
				So(rec.Body.String(), ShouldContainSubstring, "Content-Length header must be a value between 0 and 68719476736")
			})

			Convey(`Size is too large`, func() {
				rec := send("invocations/inv/artifacts/a", "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", 1<<40, "")
				So(rec.Code, ShouldEqual, http.StatusBadRequest)
				So(rec.Body.String(), ShouldContainSubstring, "Content-Length header must be a value between 0 and 68719476736")
			})

			Convey(`Update token is missing`, func() {
				rec := send("invocations/inv/artifacts/a", "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", 0, "")
				So(rec.Code, ShouldEqual, http.StatusUnauthorized)
				So(rec.Body.String(), ShouldContainSubstring, "Update-Token header is missing")
			})

			Convey(`Update token is invalid`, func() {
				rec := send("invocations/inv/artifacts/a", "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", 0, "x")
				So(rec.Code, ShouldEqual, http.StatusForbidden)
				So(rec.Body.String(), ShouldContainSubstring, "invalid Update-Token header value")
			})
		})

		tok, err := invocationTokenKind.Generate(ctx, []byte("inv"), nil, time.Hour)
		So(err, ShouldBeNil)

		Convey(`Verify state`, func() {
			Convey(`Invocation does not exist`, func() {
				rec := send("invocations/inv/artifacts/a", "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", 0, tok)
				So(rec.Code, ShouldEqual, http.StatusNotFound)
				So(rec.Body.String(), ShouldContainSubstring, "invocations/inv not found")
			})

			Convey(`Invocation is finalized`, func() {
				MustApply(ctx, InsertInvocation("inv", pb.Invocation_FINALIZED, nil))
				rec := send("invocations/inv/artifacts/a", "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", 0, tok)
				So(rec.Code, ShouldEqual, http.StatusBadRequest)
				So(rec.Body.String(), ShouldContainSubstring, "invocations/inv is not active")
			})

			Convey(`Same artifact exists`, func() {
				MustApply(ctx,
					InsertInvocation("inv", pb.Invocation_ACTIVE, nil),
					span.InsertMap("Artifacts", map[string]interface{}{
						"InvocationId": span.InvocationID("inv"),
						"ParentId":     "",
						"ArtifactId":   "a",
						"RBECASHash":   "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
						"Size":         0,
					}),
				)
				rec := send("invocations/inv/artifacts/a", "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", 0, tok)
				So(rec.Code, ShouldEqual, http.StatusNoContent)
			})

			Convey(`Different artifact exists`, func() {
				MustApply(ctx,
					InsertInvocation("inv", pb.Invocation_ACTIVE, nil),
					span.InsertMap("Artifacts", map[string]interface{}{
						"InvocationId": span.InvocationID("inv"),
						"ParentId":     "",
						"ArtifactId":   "a",
						"RBECASHash":   "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b851",
						"Size":         1,
					}),
				)
				rec := send("invocations/inv/artifacts/a", "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", 0, tok)
				So(rec.Code, ShouldEqual, http.StatusConflict)
			})
		})
	})
}
