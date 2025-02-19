// Copyright 2021 The LUCI Authors.
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

package gae

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"go.opentelemetry.io/otel/trace"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	gaebasepb "go.chromium.org/luci/server/internal/gae/base"
	remotepb "go.chromium.org/luci/server/internal/gae/remote_api"
)

func TestCall(t *testing.T) {
	t.Parallel()

	call := func(in, out proto.Message, handler http.HandlerFunc) error {
		srv := httptest.NewServer(handler)
		defer srv.Close()

		apiURL, _ := url.Parse(srv.URL + "/call")
		ctx := WithTickets(context.Background(), &Tickets{
			api:         "api-ticket",
			dapperTrace: "dapper-ticket",
			apiURL:      apiURL,
		})

		ctx = trace.ContextWithSpanContext(ctx, trace.NewSpanContext(trace.SpanContextConfig{
			TraceID: [16]byte{1, 2},
			SpanID:  [8]byte{3, 4},
		}))

		return Call(ctx, "service", "SomeMethod", in, out)
	}

	marshal := func(m proto.Message) []byte {
		b, err := proto.Marshal(m)
		if err != nil {
			panic(err)
		}
		return b
	}

	unmarshal := func(b []byte, m proto.Message) proto.Message {
		if err := proto.Unmarshal(b, m); err != nil {
			panic(err)
		}
		return m
	}

	respond := func(rw http.ResponseWriter, resp *remotepb.Response) {
		if _, err := rw.Write(marshal(resp)); err != nil {
			panic(err)
		}
	}

	req := &gaebasepb.StringProto{Value: proto.String("req")}
	res := &gaebasepb.StringProto{}

	ftt.Run("Happy path", t, func(t *ftt.Test) {
		var body []byte
		var headers http.Header

		assert.Loosely(t, call(req, res, func(rw http.ResponseWriter, req *http.Request) {
			body, _ = io.ReadAll(req.Body)
			headers = req.Header.Clone()
			respond(rw, &remotepb.Response{
				Response: marshal(&gaebasepb.StringProto{Value: proto.String("res")}),
			})
		}), should.BeNil)

		assert.Loosely(t, res, should.Match(&gaebasepb.StringProto{
			Value: proto.String("res"),
		}))

		assert.Loosely(t, unmarshal(body, &remotepb.Request{}), should.Match(&remotepb.Request{
			ServiceName: proto.String("service"),
			Method:      proto.String("SomeMethod"),
			Request:     marshal(req),
			RequestId:   proto.String("api-ticket"),
		}))

		assert.Loosely(t, headers.Get("X-Google-Rpc-Service-Endpoint"), should.Equal("app-engine-apis"))
		assert.Loosely(t, headers.Get("X-Google-Rpc-Service-Method"), should.Equal("/VMRemoteAPI.CallRemoteAPI"))
		assert.Loosely(t, headers.Get("X-Google-Rpc-Service-Deadline"), should.NotBeEmpty)
		assert.Loosely(t, headers.Get("Content-Type"), should.Equal("application/octet-stream"))
		assert.Loosely(t, headers.Get("X-Google-Dappertraceinfo"), should.Equal("dapper-ticket"))
		assert.Loosely(t, headers.Get("X-Cloud-Trace-Context"), should.Equal("01020000000000000000000000000000/217298682020626432;o=0"))
	})

	ftt.Run("Bad HTTP status code", t, func(t *ftt.Test) {
		err := call(req, res, func(rw http.ResponseWriter, req *http.Request) {
			rw.WriteHeader(404)
		})
		assert.Loosely(t, err, should.ErrLike("unexpected HTTP 404"))
	})

	ftt.Run("RPC error", t, func(t *ftt.Test) {
		err := call(req, res, func(rw http.ResponseWriter, req *http.Request) {
			respond(rw, &remotepb.Response{
				RpcError: &remotepb.RpcError{
					Code:   proto.Int32(int32(remotepb.RpcError_CALL_NOT_FOUND)),
					Detail: proto.String("boo"),
				},
			})
		})
		assert.Loosely(t, err, should.ErrLike("RPC error CALL_NOT_FOUND calling service.SomeMethod: boo"))
	})

	ftt.Run("Application error", t, func(t *ftt.Test) {
		err := call(req, res, func(rw http.ResponseWriter, req *http.Request) {
			respond(rw, &remotepb.Response{
				ApplicationError: &remotepb.ApplicationError{
					Code:   proto.Int32(123),
					Detail: proto.String("boo"),
				},
			})
		})
		assert.Loosely(t, err, should.ErrLike("API error 123 calling service.SomeMethod: boo"))
	})

	ftt.Run("Exception error", t, func(t *ftt.Test) {
		err := call(req, res, func(rw http.ResponseWriter, req *http.Request) {
			respond(rw, &remotepb.Response{
				Exception: []byte{1},
			})
		})
		assert.Loosely(t, err, should.ErrLike("service bridge returned unexpected exception from service.SomeMethod"))
	})
}
