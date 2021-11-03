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
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"google.golang.org/protobuf/proto"

	gaebasepb "go.chromium.org/luci/server/internal/gae/base"
	remotepb "go.chromium.org/luci/server/internal/gae/remote_api"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
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
			cloudTrace:  "cloud-ticket",
			apiURL:      apiURL,
		})

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

	Convey("Happy path", t, func() {
		var body []byte
		var headers http.Header

		So(call(req, res, func(rw http.ResponseWriter, req *http.Request) {
			body, _ = ioutil.ReadAll(req.Body)
			headers = req.Header.Clone()
			respond(rw, &remotepb.Response{
				Response: marshal(&gaebasepb.StringProto{Value: proto.String("res")}),
			})
		}), ShouldBeNil)

		So(res, ShouldResembleProto, &gaebasepb.StringProto{
			Value: proto.String("res"),
		})

		So(unmarshal(body, &remotepb.Request{}), ShouldResembleProto, &remotepb.Request{
			ServiceName: proto.String("service"),
			Method:      proto.String("SomeMethod"),
			Request:     marshal(req),
			RequestId:   proto.String("api-ticket"),
		})

		So(headers.Get("X-Google-Rpc-Service-Endpoint"), ShouldEqual, "app-engine-apis")
		So(headers.Get("X-Google-Rpc-Service-Method"), ShouldEqual, "/VMRemoteAPI.CallRemoteAPI")
		So(headers.Get("X-Google-Rpc-Service-Deadline"), ShouldNotBeEmpty)
		So(headers.Get("Content-Type"), ShouldEqual, "application/octet-stream")
		So(headers.Get("X-Google-Dappertraceinfo"), ShouldEqual, "dapper-ticket")
		So(headers.Get("X-Cloud-Trace-Context"), ShouldEqual, "cloud-ticket")
	})

	Convey("Bad HTTP status code", t, func() {
		err := call(req, res, func(rw http.ResponseWriter, req *http.Request) {
			rw.WriteHeader(404)
		})
		So(err, ShouldErrLike, "unexpected HTTP 404")
	})

	Convey("RPC error", t, func() {
		err := call(req, res, func(rw http.ResponseWriter, req *http.Request) {
			respond(rw, &remotepb.Response{
				RpcError: &remotepb.RpcError{
					Code:   proto.Int32(int32(remotepb.RpcError_CALL_NOT_FOUND)),
					Detail: proto.String("boo"),
				},
			})
		})
		So(err, ShouldErrLike, "RPC error CALL_NOT_FOUND calling service.SomeMethod: boo")
	})

	Convey("Application error", t, func() {
		err := call(req, res, func(rw http.ResponseWriter, req *http.Request) {
			respond(rw, &remotepb.Response{
				ApplicationError: &remotepb.ApplicationError{
					Code:   proto.Int32(123),
					Detail: proto.String("boo"),
				},
			})
		})
		So(err, ShouldErrLike, "API error 123 calling service.SomeMethod: boo")
	})

	Convey("Exception error", t, func() {
		err := call(req, res, func(rw http.ResponseWriter, req *http.Request) {
			respond(rw, &remotepb.Response{
				Exception: []byte{1},
			})
		})
		So(err, ShouldErrLike, "service bridge returned unexpected exception from service.SomeMethod")
	})
}
