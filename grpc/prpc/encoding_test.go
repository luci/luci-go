// Copyright 2016 The LUCI Authors.
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

package prpc

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/golang/protobuf/proto"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/memlogger"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestEncoding(t *testing.T) {
	t.Parallel()

	Convey("responseFormat", t, func() {
		test := func(acceptHeader string, expectedFormat Format, expectedErr interface{}) {
			acceptHeader = strings.Replace(acceptHeader, "{json}", mtPRPCJSONPB, -1)
			acceptHeader = strings.Replace(acceptHeader, "{binary}", mtPRPCBinary, -1)
			acceptHeader = strings.Replace(acceptHeader, "{text}", mtPRPCText, -1)

			Convey("Accept: "+acceptHeader, func() {
				actualFormat, err := responseFormat(acceptHeader)
				So(err, ShouldErrLike, expectedErr)
				if err == nil {
					So(actualFormat, ShouldEqual, expectedFormat)
				}
			})
		}

		test("", FormatBinary, nil)
		test(ContentTypePRPC, FormatBinary, nil)
		test(mtPRPCBinary, FormatBinary, nil)
		test(mtPRPCJSONPBLegacy, FormatJSONPB, nil)
		test(mtPRPCText, FormatText, nil)
		test(ContentTypeJSON, FormatJSONPB, nil)

		test("application/*", FormatBinary, nil)
		test("*/*", FormatBinary, nil)

		// test cases with multiple types
		test("{json},{binary}", FormatBinary, nil)
		test("{json},{binary};q=0.9", FormatJSONPB, nil)
		test("{json};q=1,{binary};q=0.9", FormatJSONPB, nil)
		test("{json},{text}", FormatJSONPB, nil)
		test("{json};q=0.9,{text}", FormatText, nil)
		test("{binary},{json},{text}", FormatBinary, nil)

		test("{json},{binary},*/*", FormatBinary, nil)
		test("{json},{binary},*/*;q=0.9", FormatBinary, nil)
		test("{json},{binary},*/*;x=y", FormatBinary, nil)
		test("{json},{binary};q=0.9,*/*", FormatBinary, nil)
		test("{json},{binary};q=0.9,*/*;q=0.8", FormatJSONPB, nil)

		// supported and unsupported mix
		test("{json},foo/bar", FormatJSONPB, nil)
		test("{json};q=0.1,foo/bar", FormatJSONPB, nil)
		test("foo/bar;q=0.1,{json}", FormatJSONPB, nil)

		// only unsupported types
		const err406 = "pRPC: Accept header: specified media types are not not supported"
		test(ContentTypePRPC+"; boo=true", 0, err406)
		test(ContentTypePRPC+"; encoding=blah", 0, err406)
		test("x", 0, err406)
		test("x,y", 0, err406)

		test("x//y", 0, "pRPC: Accept header: expected token after slash")
	})

	Convey("writeMessage", t, func() {
		msg := &HelloReply{Message: "Hi"}
		c := context.Background()

		test := func(f Format, body []byte, contentType string) {
			Convey(contentType, func() {
				rec := httptest.NewRecorder()
				writeMessage(c, rec, msg, f)
				So(rec.Code, ShouldEqual, http.StatusOK)
				So(rec.Header().Get(HeaderGRPCCode), ShouldEqual, "0")
				So(rec.Header().Get(headerContentType), ShouldEqual, contentType)
				So(rec.Header().Get("X-Content-Type-Options"), ShouldEqual, "nosniff")
				So(rec.Body.Bytes(), ShouldResemble, body)
			})
		}

		msgBytes, err := proto.Marshal(msg)
		So(err, ShouldBeNil)

		test(FormatBinary, msgBytes, mtPRPCBinary)
		test(FormatJSONPB, []byte(JSONPBPrefix+"{\"message\":\"Hi\"}\n"), mtPRPCJSONPB)
		test(FormatText, []byte("message: \"Hi\"\n"), mtPRPCText)
	})

	Convey("writeError", t, func() {
		c := context.Background()
		c = memlogger.Use(c)
		log := logging.Get(c).(*memlogger.MemLogger)

		rec := httptest.NewRecorder()

		Convey("client error", func() {
			writeError(c, rec, status.Error(codes.NotFound, "not found"))
			So(rec.Code, ShouldEqual, http.StatusNotFound)
			So(rec.Header().Get(HeaderGRPCCode), ShouldEqual, "5")
			So(rec.Header().Get(headerContentType), ShouldEqual, "text/plain")
			So(rec.Body.String(), ShouldEqual, "not found\n")
			So(log, memlogger.ShouldHaveLog, logging.Warning, "prpc: responding with NotFound error: not found")
		})

		Convey("internal error", func() {
			writeError(c, rec, status.Error(codes.Internal, "errmsg"))
			So(rec.Code, ShouldEqual, http.StatusInternalServerError)
			So(rec.Header().Get(HeaderGRPCCode), ShouldEqual, "13")
			So(rec.Header().Get(headerContentType), ShouldEqual, "text/plain")
			So(rec.Body.String(), ShouldEqual, "Internal Server Error\n")
			So(log, memlogger.ShouldHaveLog, logging.Error, "prpc: responding with Internal error: errmsg")
		})

		Convey("unknown error", func() {
			writeError(c, rec, status.Error(codes.Unknown, "errmsg"))
			So(rec.Code, ShouldEqual, http.StatusInternalServerError)
			So(rec.Header().Get(HeaderGRPCCode), ShouldEqual, "2")
			So(rec.Header().Get(headerContentType), ShouldEqual, "text/plain")
			So(rec.Body.String(), ShouldEqual, "Internal Server Error\n")
			So(log, memlogger.ShouldHaveLog, logging.Error, "prpc: responding with Unknown error: errmsg")
		})
	})
}
