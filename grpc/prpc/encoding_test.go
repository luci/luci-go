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

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/memlogger"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/convey"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/grpc/prpc/internal/testpb"
)

func TestEncoding(t *testing.T) {
	t.Parallel()

	ftt.Run("responseFormat", t, func(t *ftt.Test) {
		test := func(acceptHeader string, expectedFormat Format, expectedErr any) {
			acceptHeader = strings.Replace(acceptHeader, "{json}", mtPRPCJSONPB, -1)
			acceptHeader = strings.Replace(acceptHeader, "{binary}", mtPRPCBinary, -1)
			acceptHeader = strings.Replace(acceptHeader, "{text}", mtPRPCText, -1)

			t.Run("Accept: "+acceptHeader, func(t *ftt.Test) {
				actualFormat, protoErr := responseFormat(acceptHeader)
				if expectedErr == nil {
					assert.That(t, protoErr, should.Equal[*protocolError](nil))
				} else {
					assert.That(t, error(protoErr), should.ErrLike(expectedErr))
				}
				if protoErr == nil {
					assert.Loosely(t, actualFormat, should.Equal(expectedFormat))
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

	ftt.Run("writeResponse", t, func(t *ftt.Test) {
		msg := &testpb.HelloReply{Message: "Hi"}
		c := context.Background()

		test := func(codec protoCodec, body []byte, contentType string) {
			t.Run(contentType, func(t *ftt.Test) {
				rec := httptest.NewRecorder()
				writeResponse(c, rec, &response{
					out:   msg,
					codec: codec,
				})
				assert.Loosely(t, rec.Code, should.Equal(http.StatusOK))
				assert.Loosely(t, rec.Header().Get(HeaderGRPCCode), should.Equal("0"))
				assert.Loosely(t, rec.Header().Get(headerContentType), should.Equal(contentType))
				assert.Loosely(t, rec.Body.Bytes(), should.Resemble(body))
			})
		}

		msgBytes, err := proto.Marshal(msg)
		assert.Loosely(t, err, should.BeNil)

		test(codecWireV1, msgBytes, mtPRPCBinary)
		test(codecJSONV1, []byte(JSONPBPrefix+"{\"message\":\"Hi\"}\n"), mtPRPCJSONPB)
		test(codecTextV1, []byte("message: \"Hi\"\n"), mtPRPCText)

		t.Run("compression", func(t *ftt.Test) {
			rec := httptest.NewRecorder()
			msg := &testpb.HelloReply{Message: strings.Repeat("A", 1024)}
			writeResponse(c, rec, &response{
				out:         msg,
				codec:       codecTextV1,
				acceptsGZip: true,
			})
			assert.Loosely(t, rec.Code, should.Equal(http.StatusOK))
			assert.Loosely(t, rec.Header().Get("Content-Encoding"), should.Equal("gzip"))
			assert.Loosely(t, rec.Body.Len(), should.BeLessThan(1024))
		})

		t.Run("maxResponseSize", func(t *ftt.Test) {
			rec := httptest.NewRecorder()
			msg := &testpb.HelloReply{Message: strings.Repeat("A", 1024)}
			writeResponse(c, rec, &response{
				out:             msg,
				codec:           codecJSONV1,
				maxResponseSize: 123,
			})
			assert.Loosely(t, rec.Code, should.Equal(http.StatusServiceUnavailable))
			assert.Loosely(t, rec.Header().Get(HeaderGRPCCode), should.Equal("14")) // codes.Unavailable
			assert.Loosely(t, rec.Header().Get(HeaderStatusDetail), should.NotBeEmpty)
			body, _ := io.ReadAll(rec.Body)
			assert.Loosely(t, string(body), should.HaveSuffix("exceeds the client limit 123\n"))
		})
	})

	ftt.Run("writeError", t, func(t *ftt.Test) {
		c := context.Background()
		c = memlogger.Use(c)
		log := logging.Get(c).(*memlogger.MemLogger)

		rec := httptest.NewRecorder()

		t.Run("client error", func(t *ftt.Test) {
			writeError(c, rec, status.Error(codes.NotFound, "not found"), codecWireV1)
			assert.Loosely(t, rec.Code, should.Equal(http.StatusNotFound))
			assert.Loosely(t, rec.Header().Get(HeaderGRPCCode), should.Equal("5"))
			assert.Loosely(t, rec.Header().Get(headerContentType), should.Equal("text/plain; charset=utf-8"))
			assert.Loosely(t, rec.Body.String(), should.Equal("not found\n"))
			assert.Loosely(t, log, convey.Adapt(memlogger.ShouldHaveLog)(logging.Warning, "prpc: responding with NotFound error (HTTP 404): not found"))
		})

		t.Run("internal error", func(t *ftt.Test) {
			writeError(c, rec, status.Error(codes.Internal, "errmsg"), codecWireV1)
			assert.Loosely(t, rec.Code, should.Equal(http.StatusInternalServerError))
			assert.Loosely(t, rec.Header().Get(HeaderGRPCCode), should.Equal("13"))
			assert.Loosely(t, rec.Header().Get(headerContentType), should.Equal("text/plain; charset=utf-8"))
			assert.Loosely(t, rec.Body.String(), should.Equal("Internal server error\n"))
			assert.Loosely(t, log, convey.Adapt(memlogger.ShouldHaveLog)(logging.Error, "prpc: responding with Internal error (HTTP 500): errmsg"))
		})

		t.Run("unknown error", func(t *ftt.Test) {
			writeError(c, rec, status.Error(codes.Unknown, "errmsg"), codecWireV1)
			assert.Loosely(t, rec.Code, should.Equal(http.StatusInternalServerError))
			assert.Loosely(t, rec.Header().Get(HeaderGRPCCode), should.Equal("2"))
			assert.Loosely(t, rec.Header().Get(headerContentType), should.Equal("text/plain; charset=utf-8"))
			assert.Loosely(t, rec.Body.String(), should.Equal("Unknown server error\n"))
			assert.Loosely(t, log, convey.Adapt(memlogger.ShouldHaveLog)(logging.Error, "prpc: responding with Unknown error (HTTP 500): errmsg"))
		})

		t.Run("status details", func(t *ftt.Test) {
			testStatusDetails := func(codec protoCodec, expected []string) {
				st := status.New(codes.InvalidArgument, "invalid argument")

				st, err := st.WithDetails(&errdetails.BadRequest{
					FieldViolations: []*errdetails.BadRequest_FieldViolation{
						{Field: "a"},
					},
				})
				assert.Loosely(t, err, should.BeNil)

				st, err = st.WithDetails(&errdetails.Help{
					Links: []*errdetails.Help_Link{
						{Url: "https://example.com"},
					},
				})
				assert.Loosely(t, err, should.BeNil)

				writeError(c, rec, st.Err(), codec)
				assert.Loosely(t, rec.Header()[HeaderStatusDetail], should.Resemble(expected))
			}

			t.Run("binary", func(t *ftt.Test) {
				testStatusDetails(codecWireV1, []string{
					"Cil0eXBlLmdvb2dsZWFwaXMuY29tL2dvb2dsZS5ycGMuQmFkUmVxdWVzdBIFCgMKAWE=",
					"CiN0eXBlLmdvb2dsZWFwaXMuY29tL2dvb2dsZS5ycGMuSGVscBIXChUSE2h0dHBzOi8vZXhhbXBsZS5jb20=",
				})
			})

			t.Run("json", func(t *ftt.Test) {
				testStatusDetails(codecJSONV1, []string{
					"eyJAdHlwZSI6InR5cGUuZ29vZ2xlYXBpcy5jb20vZ29vZ2xlLnJwYy5CYWRSZXF1ZXN0IiwiZmllbGRWaW9sYXRpb25zIjpbeyJmaWVsZCI6ImEifV19",
					"eyJAdHlwZSI6InR5cGUuZ29vZ2xlYXBpcy5jb20vZ29vZ2xlLnJwYy5IZWxwIiwibGlua3MiOlt7InVybCI6Imh0dHBzOi8vZXhhbXBsZS5jb20ifV19",
				})
			})

			t.Run("text", func(t *ftt.Test) {
				testStatusDetails(codecTextV1, []string{
					"dHlwZV91cmw6ICJ0eXBlLmdvb2dsZWFwaXMuY29tL2dvb2dsZS5ycGMuQmFkUmVxdWVzdCIKdmFsdWU6ICJcblwwMDNcblwwMDFhIgo=",
					"dHlwZV91cmw6ICJ0eXBlLmdvb2dsZWFwaXMuY29tL2dvb2dsZS5ycGMuSGVscCIKdmFsdWU6ICJcblwwMjVcMDIyXDAyM2h0dHBzOi8vZXhhbXBsZS5jb20iCg==",
				})
			})
		})
	})
}
