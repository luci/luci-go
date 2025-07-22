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
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

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

		test := func(codec protoCodec, contentType string, expected []byte) {
			t.Run(contentType, func(t *ftt.Test) {
				rec := httptest.NewRecorder()
				writeResponse(c, rec, &response{
					out:   msg,
					codec: codec,
				})
				assert.Loosely(t, rec.Code, should.Equal(http.StatusOK))
				assert.Loosely(t, rec.Header().Get(HeaderGRPCCode), should.Equal("0"))
				assert.Loosely(t, rec.Header().Get(headerContentType), should.Equal(contentType))
				assert.That(t, normalizeEnc(codec, rec.Body.Bytes()), should.Match(normalizeEnc(codec, expected)))
			})
		}

		wirePB, err := codecWireV2.Encode(nil, msg)
		assert.NoErr(t, err)

		test(codecWireV2, mtPRPCBinary, wirePB)
		test(codecJSONV2, mtPRPCJSONPB, []byte(")]}'\n{\"message\":\"Hi\"}\n"))
		test(codecTextV2, mtPRPCText, []byte(`message:"Hi"`))

		t.Run("compression", func(t *ftt.Test) {
			rec := httptest.NewRecorder()
			msg := &testpb.HelloReply{Message: strings.Repeat("A", 1024)}
			writeResponse(c, rec, &response{
				out:         msg,
				codec:       codecTextV2,
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
				codec:           codecJSONV2,
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
			writeError(c, rec, status.Error(codes.NotFound, "not found"), codecWireV2)
			assert.Loosely(t, rec.Code, should.Equal(http.StatusNotFound))
			assert.Loosely(t, rec.Header().Get(HeaderGRPCCode), should.Equal("5"))
			assert.Loosely(t, rec.Header().Get(headerContentType), should.Equal("text/plain; charset=utf-8"))
			assert.Loosely(t, rec.Body.String(), should.Equal("not found\n"))
			assert.Loosely(t, log, convey.Adapt(memlogger.ShouldHaveLog)(logging.Warning, "prpc: responding with NotFound error (HTTP 404): not found"))
		})

		t.Run("internal error", func(t *ftt.Test) {
			writeError(c, rec, status.Error(codes.Internal, "errmsg"), codecWireV2)
			assert.Loosely(t, rec.Code, should.Equal(http.StatusInternalServerError))
			assert.Loosely(t, rec.Header().Get(HeaderGRPCCode), should.Equal("13"))
			assert.Loosely(t, rec.Header().Get(headerContentType), should.Equal("text/plain; charset=utf-8"))
			assert.Loosely(t, rec.Body.String(), should.Equal("Internal server error\n"))
			assert.Loosely(t, log, convey.Adapt(memlogger.ShouldHaveLog)(logging.Error, "prpc: responding with Internal error (HTTP 500): errmsg"))
		})

		t.Run("unknown error", func(t *ftt.Test) {
			writeError(c, rec, status.Error(codes.Unknown, "errmsg"), codecWireV2)
			assert.Loosely(t, rec.Code, should.Equal(http.StatusInternalServerError))
			assert.Loosely(t, rec.Header().Get(HeaderGRPCCode), should.Equal("2"))
			assert.Loosely(t, rec.Header().Get(headerContentType), should.Equal("text/plain; charset=utf-8"))
			assert.Loosely(t, rec.Body.String(), should.Equal("Unknown server error\n"))
			assert.Loosely(t, log, convey.Adapt(memlogger.ShouldHaveLog)(logging.Error, "prpc: responding with Unknown error (HTTP 500): errmsg"))
		})

		t.Run("status details", func(t *ftt.Test) {
			errDetails1 := &errdetails.BadRequest{
				FieldViolations: []*errdetails.BadRequest_FieldViolation{
					{Field: "a"},
				},
			}
			errDetails2 := &errdetails.Help{
				Links: []*errdetails.Help_Link{
					{Url: "https://example.com"},
				},
			}

			testStatusDetails := func(codec protoCodec, expected [][]byte) {
				st := status.New(codes.InvalidArgument, "invalid argument")

				st, err := st.WithDetails(errDetails1)
				assert.Loosely(t, err, should.BeNil)

				st, err = st.WithDetails(errDetails2)
				assert.Loosely(t, err, should.BeNil)

				writeError(c, rec, st.Err(), codec)

				assert.Loosely(t, rec.Header()[HeaderStatusDetail], should.HaveLength(len(expected)))
				for i, val := range rec.Header()[HeaderStatusDetail] {
					blob, err := base64.StdEncoding.DecodeString(val)
					assert.That(t, err, should.ErrLike(nil))
					assert.That(t, normalizeEnc(codec, blob), should.Match(normalizeEnc(codec, expected[i])))
				}
			}

			wireAnyPB := func(msg proto.Message) []byte {
				apb, err := anypb.New(msg)
				if err != nil {
					panic(apb)
				}
				blob, err := codecWireV2.Encode(nil, apb)
				if err != nil {
					panic(err)
				}
				return blob
			}

			t.Run("binary", func(t *ftt.Test) {
				testStatusDetails(codecWireV2, [][]byte{
					wireAnyPB(errDetails1),
					wireAnyPB(errDetails2),
				})
			})

			t.Run("json", func(t *ftt.Test) {
				testStatusDetails(codecJSONV2, [][]byte{
					[]byte(`{"@type":"type.googleapis.com/google.rpc.BadRequest","fieldViolations":[{"field":"a"}]}`),
					[]byte(`{"@type":"type.googleapis.com/google.rpc.Help","links":[{"url":"https://example.com"}]}`),
				})
			})

			t.Run("text", func(t *ftt.Test) {
				testStatusDetails(codecTextV2, [][]byte{
					[]byte(`[type.googleapis.com/google.rpc.BadRequest]:{field_violations:{field:"a"}}`),
					[]byte(`[type.googleapis.com/google.rpc.Help]:{links:{url:"https://example.com"}}`),
				})
			})
		})
	})
}

// normalizeEnc does lossy normalization of encoded data for comparisons.
func normalizeEnc(codec protoCodec, blob []byte) []byte {
	switch codec {
	case codecWireV2:
		return blob
	case codecJSONV2, codecTextV2:
		return bytes.Replace(blob, []byte(" "), nil, -1)
	default:
		panic("impossible")
	}
}
