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
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/golang/protobuf/proto"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
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
		test := func(acceptHeader string, expectedFormat Format, expectedErr any) {
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
		const err406 = "prpc: bad Accept header: specified media types are not not supported"
		test(ContentTypePRPC+"; boo=true", 0, err406)
		test(ContentTypePRPC+"; encoding=blah", 0, err406)
		test("x", 0, err406)
		test("x,y", 0, err406)

		test("x//y", 0, "prpc: bad Accept header: specified media types are not not supported")
	})

	Convey("writeResponse", t, func() {
		msg := &HelloReply{Message: "Hi"}
		c := context.Background()

		test := func(f Format, body []byte, contentType string) {
			Convey(contentType, func() {
				rec := httptest.NewRecorder()
				writeResponse(c, rec, &response{
					out: msg,
					fmt: f,
				})
				So(rec.Code, ShouldEqual, http.StatusOK)
				So(rec.Header().Get(HeaderGRPCCode), ShouldEqual, "0")
				So(rec.Header().Get(headerContentType), ShouldEqual, contentType)
				So(rec.Body.Bytes(), ShouldResemble, body)
			})
		}

		msgBytes, err := proto.Marshal(msg)
		So(err, ShouldBeNil)

		test(FormatBinary, msgBytes, mtPRPCBinary)
		test(FormatJSONPB, []byte(JSONPBPrefix+"{\"message\":\"Hi\"}\n"), mtPRPCJSONPB)
		test(FormatText, []byte("message: \"Hi\"\n"), mtPRPCText)

		Convey("compression", func() {
			rec := httptest.NewRecorder()
			msg := &HelloReply{Message: strings.Repeat("A", 1024)}
			writeResponse(c, rec, &response{
				out:         msg,
				fmt:         FormatText,
				acceptsGZip: true,
			})
			So(rec.Code, ShouldEqual, http.StatusOK)
			So(rec.Header().Get("Content-Encoding"), ShouldEqual, "gzip")
			So(rec.Body.Len(), ShouldBeLessThan, 1024)
		})

		Convey("maxResponseSize", func() {
			rec := httptest.NewRecorder()
			msg := &HelloReply{Message: strings.Repeat("A", 1024)}
			writeResponse(c, rec, &response{
				out:             msg,
				fmt:             FormatJSONPB,
				maxResponseSize: 123,
			})
			So(rec.Code, ShouldEqual, http.StatusServiceUnavailable)
			So(rec.Header().Get(HeaderGRPCCode), ShouldEqual, "14") // codes.Unavailable
			So(rec.Header().Get(HeaderStatusDetail), ShouldNotBeEmpty)
			body, _ := io.ReadAll(rec.Body)
			So(string(body), ShouldEndWith, "exceeds the client limit 123\n")
		})
	})

	Convey("writeError", t, func() {
		c := context.Background()
		c = memlogger.Use(c)
		log := logging.Get(c).(*memlogger.MemLogger)

		rec := httptest.NewRecorder()

		Convey("client error", func() {
			writeError(c, rec, status.Error(codes.NotFound, "not found"), FormatBinary)
			So(rec.Code, ShouldEqual, http.StatusNotFound)
			So(rec.Header().Get(HeaderGRPCCode), ShouldEqual, "5")
			So(rec.Header().Get(headerContentType), ShouldEqual, "text/plain; charset=utf-8")
			So(rec.Body.String(), ShouldEqual, "not found\n")
			So(log, memlogger.ShouldHaveLog, logging.Warning, "prpc: responding with NotFound error (HTTP 404): not found")
		})

		Convey("internal error", func() {
			writeError(c, rec, status.Error(codes.Internal, "errmsg"), FormatBinary)
			So(rec.Code, ShouldEqual, http.StatusInternalServerError)
			So(rec.Header().Get(HeaderGRPCCode), ShouldEqual, "13")
			So(rec.Header().Get(headerContentType), ShouldEqual, "text/plain; charset=utf-8")
			So(rec.Body.String(), ShouldEqual, "Internal server error\n")
			So(log, memlogger.ShouldHaveLog, logging.Error, "prpc: responding with Internal error (HTTP 500): errmsg")
		})

		Convey("unknown error", func() {
			writeError(c, rec, status.Error(codes.Unknown, "errmsg"), FormatBinary)
			So(rec.Code, ShouldEqual, http.StatusInternalServerError)
			So(rec.Header().Get(HeaderGRPCCode), ShouldEqual, "2")
			So(rec.Header().Get(headerContentType), ShouldEqual, "text/plain; charset=utf-8")
			So(rec.Body.String(), ShouldEqual, "Unknown server error\n")
			So(log, memlogger.ShouldHaveLog, logging.Error, "prpc: responding with Unknown error (HTTP 500): errmsg")
		})

		Convey("status details", func() {
			testStatusDetails := func(format Format, expected []string) {
				st := status.New(codes.InvalidArgument, "invalid argument")

				st, err := st.WithDetails(&errdetails.BadRequest{
					FieldViolations: []*errdetails.BadRequest_FieldViolation{
						{Field: "a"},
					},
				})
				So(err, ShouldBeNil)

				st, err = st.WithDetails(&errdetails.Help{
					Links: []*errdetails.Help_Link{
						{Url: "https://example.com"},
					},
				})
				So(err, ShouldBeNil)

				writeError(c, rec, st.Err(), format)
				So(rec.Header()[HeaderStatusDetail], ShouldResemble, expected)
			}

			Convey("binary", func() {
				testStatusDetails(FormatBinary, []string{
					"Cil0eXBlLmdvb2dsZWFwaXMuY29tL2dvb2dsZS5ycGMuQmFkUmVxdWVzdBIFCgMKAWE=",
					"CiN0eXBlLmdvb2dsZWFwaXMuY29tL2dvb2dsZS5ycGMuSGVscBIXChUSE2h0dHBzOi8vZXhhbXBsZS5jb20=",
				})
			})

			Convey("json", func() {
				testStatusDetails(FormatJSONPB, []string{
					"eyJAdHlwZSI6InR5cGUuZ29vZ2xlYXBpcy5jb20vZ29vZ2xlLnJwYy5CYWRSZXF1ZXN0IiwiZmllbGRWaW9sYXRpb25zIjpbeyJmaWVsZCI6ImEifV19",
					"eyJAdHlwZSI6InR5cGUuZ29vZ2xlYXBpcy5jb20vZ29vZ2xlLnJwYy5IZWxwIiwibGlua3MiOlt7InVybCI6Imh0dHBzOi8vZXhhbXBsZS5jb20ifV19",
				})
			})

			Convey("text", func() {
				testStatusDetails(FormatText, []string{
					"dHlwZV91cmw6ICJ0eXBlLmdvb2dsZWFwaXMuY29tL2dvb2dsZS5ycGMuQmFkUmVxdWVzdCIKdmFsdWU6ICJcblwwMDNcblwwMDFhIgo=",
					"dHlwZV91cmw6ICJ0eXBlLmdvb2dsZWFwaXMuY29tL2dvb2dsZS5ycGMuSGVscCIKdmFsdWU6ICJcblwwMjVcMDIyXDAyM2h0dHBzOi8vZXhhbXBsZS5jb20iCg==",
				})
			})
		})
	})
}
