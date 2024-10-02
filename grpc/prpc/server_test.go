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
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/golang/protobuf/proto"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/prototext"

	"go.chromium.org/luci/server/router"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

type greeterService struct {
	headerMD   metadata.MD
	errDetails []proto.Message
}

func (s *greeterService) SayHello(ctx context.Context, req *HelloRequest) (*HelloReply, error) {
	if req.Name == "" {
		return nil, status.Errorf(codes.InvalidArgument, "Name unspecified")
	}

	if len(s.errDetails) > 0 {
		st, err := status.New(codes.Unknown, "").WithDetails(s.errDetails...)
		if err == nil {
			err = st.Err()
		}
		return nil, err
	}

	if s.headerMD != nil {
		SetHeader(ctx, s.headerMD)
	}

	return &HelloReply{
		Message: "Hello " + req.Name,
	}, nil
}

type calcService struct{}

func (s *calcService) Multiply(ctx context.Context, req *MultiplyRequest) (*MultiplyResponse, error) {
	return &MultiplyResponse{
		Z: req.X & req.Y,
	}, nil
}

func decodeRequest(body []byte) *HelloRequest {
	msg := &HelloRequest{}
	if err := prototext.Unmarshal(body, msg); err != nil {
		panic(err)
	}
	return msg
}

func decodeReply(body []byte) *HelloReply {
	msg := &HelloReply{}
	if err := prototext.Unmarshal(body, msg); err != nil {
		panic(err)
	}
	return msg
}

func TestServer(t *testing.T) {
	t.Parallel()

	Convey("Greeter service", t, func() {
		server := Server{MaxRequestSize: 100}

		greeterSvc := &greeterService{}
		RegisterGreeterServer(&server, greeterSvc)

		Convey("Register Calc service", func() {
			RegisterCalcServer(&server, &calcService{})
			So(server.ServiceNames(), ShouldResemble, []string{
				"prpc.Calc",
				"prpc.Greeter",
			})
		})

		Convey("Handlers", func() {
			c := context.Background()
			r := router.New()
			server.InstallHandlers(r, router.NewMiddlewareChain(
				func(ctx *router.Context, next router.Handler) {
					ctx.Request = ctx.Request.WithContext(c)
					next(ctx)
				},
			))
			res := httptest.NewRecorder()
			hiMsg := bytes.NewBufferString(`name: "Lucy"`)
			req, err := http.NewRequest("POST", "/prpc/prpc.Greeter/SayHello", hiMsg)
			So(err, ShouldBeNil)
			req.Header.Set("Content-Type", mtPRPCText)

			strCode := func(c codes.Code) string {
				return strconv.Itoa(int(c))
			}

			Convey("Works", func() {
				req.Header.Set("Accept", mtPRPCText)
				r.ServeHTTP(res, req)
				So(res.Code, ShouldEqual, http.StatusOK)
				So(res.Result().Header, ShouldResemble, http.Header{
					"Content-Type":           {"application/prpc; encoding=text"},
					"Date":                   nil,
					"X-Content-Type-Options": {"nosniff"},
					"X-Prpc-Grpc-Code":       {strCode(codes.OK)},
				})
				So(decodeReply(res.Body.Bytes()), ShouldResembleProto, &HelloReply{Message: "Hello Lucy"})
			})

			Convey("Header Metadata", func() {
				greeterSvc.headerMD = metadata.Pairs("a", "1", "b", "2")
				r.ServeHTTP(res, req)
				So(res.Code, ShouldEqual, http.StatusOK)
				So(res.Result().Header, ShouldResemble, http.Header{
					"A":                      {"1"},
					"B":                      {"2"},
					"Content-Type":           {"application/prpc; encoding=binary"},
					"Date":                   nil,
					"X-Content-Type-Options": {"nosniff"},
					"X-Prpc-Grpc-Code":       {strCode(codes.OK)},
				})
			})

			Convey("Status details", func() {
				greeterSvc.errDetails = []proto.Message{&errdetails.DebugInfo{Detail: "x"}}
				r.ServeHTTP(res, req)
				So(res.Result().Header, ShouldResemble, http.Header{
					"Content-Type":              {"text/plain"},
					"Date":                      nil,
					"X-Content-Type-Options":    {"nosniff"},
					"X-Prpc-Grpc-Code":          {strCode(codes.Unknown)},
					"X-Prpc-Status-Details-Bin": {"Cih0eXBlLmdvb2dsZWFwaXMuY29tL2dvb2dsZS5ycGMuRGVidWdJbmZvEgMSAXg="},
				})
			})

			Convey("Invalid Accept header", func() {
				req.Header.Set("Accept", "blah")
				r.ServeHTTP(res, req)
				So(res.Code, ShouldEqual, http.StatusNotAcceptable)
				So(res.Result().Header, ShouldResemble, http.Header{
					"Content-Type":              {"text/plain"},
					"Date":                      nil,
					"X-Content-Type-Options":    {"nosniff"},
					"X-Prpc-Grpc-Code":          {strCode(codes.InvalidArgument)},
					"X-Prpc-Status-Details-Bin": []string{},
				})
			})

			Convey("Invalid max response size header", func() {
				for _, bad := range []string{"not-an-int", "0", "-1", "   123"} {
					req.Header.Set(HeaderMaxResponseSize, bad)
					r.ServeHTTP(res, req)
					So(res.Code, ShouldEqual, http.StatusBadRequest)
					So(res.Result().Header, ShouldResemble, http.Header{
						"Content-Type":              {"text/plain"},
						"Date":                      nil,
						"X-Content-Type-Options":    {"nosniff"},
						"X-Prpc-Grpc-Code":          {strCode(codes.InvalidArgument)},
						"X-Prpc-Status-Details-Bin": []string{},
					})
				}
			})

			Convey("Invalid header", func() {
				req.Header.Set("X-Bin", "zzz")
				r.ServeHTTP(res, req)
				So(res.Code, ShouldEqual, http.StatusBadRequest)
				So(res.Result().Header, ShouldResemble, http.Header{
					"Content-Type":              {"text/plain"},
					"Date":                      nil,
					"X-Content-Type-Options":    {"nosniff"},
					"X-Prpc-Grpc-Code":          {strCode(codes.InvalidArgument)},
					"X-Prpc-Status-Details-Bin": []string{},
				})
			})

			Convey("Malformed request message", func() {
				hiMsg.WriteString("\nblah")
				r.ServeHTTP(res, req)
				So(res.Code, ShouldEqual, http.StatusBadRequest)
				So(res.Result().Header, ShouldResemble, http.Header{
					"Content-Type":              {"text/plain"},
					"Date":                      nil,
					"X-Content-Type-Options":    {"nosniff"},
					"X-Prpc-Grpc-Code":          {strCode(codes.InvalidArgument)},
					"X-Prpc-Status-Details-Bin": []string{},
				})
			})

			Convey("Invalid request message", func() {
				hiMsg.Reset()
				r.ServeHTTP(res, req)
				So(res.Code, ShouldEqual, http.StatusBadRequest)
				So(res.Result().Header, ShouldResemble, http.Header{
					"Content-Type":              {"text/plain"},
					"Date":                      nil,
					"X-Content-Type-Options":    {"nosniff"},
					"X-Prpc-Grpc-Code":          {strCode(codes.InvalidArgument)},
					"X-Prpc-Status-Details-Bin": []string{},
				})
				So(res.Body.String(), ShouldEqual, "Name unspecified\n")
			})

			Convey("no such service", func() {
				req.URL.Path = "/prpc/xxx/SayHello"
				r.ServeHTTP(res, req)
				So(res.Code, ShouldEqual, http.StatusNotImplemented)
				So(res.Result().Header, ShouldResemble, http.Header{
					"Content-Type":              {"text/plain"},
					"Date":                      nil,
					"X-Content-Type-Options":    {"nosniff"},
					"X-Prpc-Grpc-Code":          {strCode(codes.Unimplemented)},
					"X-Prpc-Status-Details-Bin": []string{},
				})
			})

			Convey("no such method", func() {
				req.URL.Path = "/prpc/Greeter/xxx"
				r.ServeHTTP(res, req)
				So(res.Code, ShouldEqual, http.StatusNotImplemented)
				So(res.Result().Header, ShouldResemble, http.Header{
					"Content-Type":              {"text/plain"},
					"Date":                      nil,
					"X-Content-Type-Options":    {"nosniff"},
					"X-Prpc-Grpc-Code":          {strCode(codes.Unimplemented)},
					"X-Prpc-Status-Details-Bin": []string{},
				})
			})

			Convey(`When access control is enabled without credentials`, func() {
				server.AccessControl = func(ctx context.Context, origin string) AccessControlDecision {
					return AccessControlDecision{
						AllowCrossOriginRequests: true,
						AllowCredentials:         false,
					}
				}
				req.Header.Add("Origin", "http://example.com")

				r.ServeHTTP(res, req)
				So(res.Code, ShouldEqual, http.StatusOK)
				So(res.Result().Header, ShouldResemble, http.Header{
					"Access-Control-Allow-Origin":   {"http://example.com"},
					"Access-Control-Expose-Headers": {"X-Prpc-Grpc-Code, X-Prpc-Status-Details-Bin"},
					"Content-Type":                  {"application/prpc; encoding=binary"},
					"Date":                          nil,
					"Vary":                          {"Origin"},
					"X-Content-Type-Options":        {"nosniff"},
					"X-Prpc-Grpc-Code":              {strCode(codes.OK)},
				})
			})

			Convey(`When access control is enabled for "http://example.com"`, func() {
				decision := AccessControlDecision{
					AllowCrossOriginRequests: true,
					AllowCredentials:         true,
				}
				server.AccessControl = func(ctx context.Context, origin string) AccessControlDecision {
					if origin == "http://example.com" {
						return decision
					}
					return AccessControlDecision{}
				}

				Convey(`When sending an OPTIONS request`, func() {
					req.Method = "OPTIONS"

					Convey(`Will supply Access-* headers to "http://example.com"`, func() {
						req.Header.Add("Origin", "http://example.com")

						r.ServeHTTP(res, req)
						So(res.Code, ShouldEqual, http.StatusOK)
						So(res.Result().Header, ShouldResemble, http.Header{
							"Access-Control-Allow-Credentials": {"true"},
							"Access-Control-Allow-Headers":     {"Origin, Content-Type, Accept, Authorization, X-Prpc-Grpc-Timeout, X-Prpc-Max-Response-Size"},
							"Access-Control-Allow-Methods":     {"OPTIONS, POST"},
							"Access-Control-Allow-Origin":      {"http://example.com"},
							"Access-Control-Max-Age":           {"600"},
							"Vary":                             {"Origin"},
						})
					})

					Convey(`Will not supply access-* headers to "http://foo.bar"`, func() {
						req.Header.Add("Origin", "http://foo.bar")

						r.ServeHTTP(res, req)
						So(res.Code, ShouldEqual, http.StatusOK)
						So(res.Result().Header, ShouldResemble, http.Header{})
					})
				})

				Convey(`When sending a POST request`, func() {
					Convey(`Will supply Access-* headers to "http://example.com"`, func() {
						req.Header.Add("Origin", "http://example.com")

						r.ServeHTTP(res, req)
						So(res.Code, ShouldEqual, http.StatusOK)
						So(res.Result().Header, ShouldResemble, http.Header{
							"Access-Control-Allow-Credentials": {"true"},
							"Access-Control-Allow-Origin":      {"http://example.com"},
							"Access-Control-Expose-Headers":    {"X-Prpc-Grpc-Code, X-Prpc-Status-Details-Bin"},
							"Content-Type":                     {"application/prpc; encoding=binary"},
							"Date":                             nil,
							"Vary":                             {"Origin"},
							"X-Content-Type-Options":           {"nosniff"},
							"X-Prpc-Grpc-Code":                 {strCode(codes.OK)},
						})
					})

					Convey(`Will not supply access-* headers to "http://foo.bar"`, func() {
						req.Header.Add("Origin", "http://foo.bar")

						r.ServeHTTP(res, req)
						So(res.Code, ShouldEqual, http.StatusOK)
						So(res.Result().Header, ShouldResemble, http.Header{
							"Content-Type":           {"application/prpc; encoding=binary"},
							"Date":                   nil,
							"X-Content-Type-Options": {"nosniff"},
							"X-Prpc-Grpc-Code":       {strCode(codes.OK)},
						})
					})
				})

				Convey(`Using custom AllowHeaders`, func() {
					decision.AllowHeaders = []string{"Booboo", "bobo"}

					req.Method = "OPTIONS"
					req.Header.Add("Origin", "http://example.com")

					r.ServeHTTP(res, req)
					So(res.Code, ShouldEqual, http.StatusOK)
					So(res.Result().Header, ShouldResemble, http.Header{
						"Access-Control-Allow-Credentials": {"true"},
						"Access-Control-Allow-Headers":     {"Booboo, bobo, Origin, Content-Type, Accept, Authorization, X-Prpc-Grpc-Timeout, X-Prpc-Max-Response-Size"},
						"Access-Control-Allow-Methods":     {"OPTIONS, POST"},
						"Access-Control-Allow-Origin":      {"http://example.com"},
						"Access-Control-Max-Age":           {"600"},
						"Vary":                             {"Origin"},
					})
				})
			})

			Convey("Override callback: pass through", func() {
				req.Header.Set("Accept", mtPRPCText)

				called := false
				server.RegisterOverride("prpc.Greeter", "SayHello",
					func(rw http.ResponseWriter, req *http.Request, body func(msg proto.Message) error) (bool, error) {
						called = true
						return false, nil
					},
				)

				r.ServeHTTP(res, req)
				So(res.Code, ShouldEqual, http.StatusOK)
				So(res.Result().Header, ShouldResemble, http.Header{
					"Content-Type":           {"application/prpc; encoding=text"},
					"Date":                   nil,
					"X-Content-Type-Options": {"nosniff"},
					"X-Prpc-Grpc-Code":       {strCode(codes.OK)},
				})
				So(decodeReply(res.Body.Bytes()), ShouldResembleProto, &HelloReply{Message: "Hello Lucy"})
				So(called, ShouldBeTrue)
			})

			Convey("Override callback: override", func() {
				req.Header.Set("Accept", mtPRPCText)

				var rawBody []byte
				server.RegisterOverride("prpc.Greeter", "SayHello",
					func(rw http.ResponseWriter, req *http.Request, body func(msg proto.Message) error) (bool, error) {
						rawBody, _ = io.ReadAll(req.Body)
						rw.Header().Set("Overridden", "1")
						_, _ = fmt.Fprintf(rw, "Override")
						return true, nil
					},
				)

				r.ServeHTTP(res, req)
				So(res.Code, ShouldEqual, http.StatusOK)
				So(res.Result().Header, ShouldResemble, http.Header{
					"Content-Type": {"text/plain; charset=utf-8"},
					"Overridden":   {"1"},
				})
				So(res.Body.String(), ShouldEqual, "Override")
				So(decodeRequest(rawBody), ShouldResembleProto, &HelloRequest{Name: "Lucy"})
			})

			Convey("Override callback: error", func() {
				req.Header.Set("Accept", mtPRPCText)

				server.RegisterOverride("prpc.Greeter", "SayHello",
					func(rw http.ResponseWriter, req *http.Request, body func(msg proto.Message) error) (bool, error) {
						_, _ = io.ReadAll(req.Body)
						return false, errors.New("BOOM")
					},
				)

				r.ServeHTTP(res, req)
				So(res.Code, ShouldEqual, http.StatusBadRequest)
				So(res.Result().Header, ShouldResemble, http.Header{
					"Content-Type":              {"text/plain"},
					"X-Prpc-Grpc-Code":          {strCode(codes.InvalidArgument)},
					"X-Prpc-Status-Details-Bin": []string{},
				})
				So(res.Body.String(), ShouldEqual, "the override check: BOOM\n")
			})

			Convey("Override callback: peek, pass through", func() {
				req.Header.Set("Accept", mtPRPCText)

				var rpcReq *HelloRequest
				server.RegisterOverride("prpc.Greeter", "SayHello",
					func(rw http.ResponseWriter, req *http.Request, body func(msg proto.Message) error) (bool, error) {
						r := &HelloRequest{}
						if err := body(r); err != nil {
							return false, err
						}
						rpcReq = r
						return false, nil
					},
				)

				r.ServeHTTP(res, req)
				So(res.Code, ShouldEqual, http.StatusOK)
				So(res.Result().Header, ShouldResemble, http.Header{
					"Content-Type":           {"application/prpc; encoding=text"},
					"Date":                   nil,
					"X-Content-Type-Options": {"nosniff"},
					"X-Prpc-Grpc-Code":       {strCode(codes.OK)},
				})
				So(decodeReply(res.Body.Bytes()), ShouldResembleProto, &HelloReply{Message: "Hello Lucy"})
				So(rpcReq, ShouldResembleProto, &HelloRequest{Name: "Lucy"})
			})

			Convey("Override callback: peek, override", func() {
				req.Header.Set("Accept", mtPRPCText)

				var rpcReq *HelloRequest
				var rawBody []byte
				server.RegisterOverride("prpc.Greeter", "SayHello",
					func(rw http.ResponseWriter, req *http.Request, body func(msg proto.Message) error) (bool, error) {
						r := &HelloRequest{}
						if err := body(r); err != nil {
							return false, err
						}
						rpcReq = r

						rw.Header().Set("Overridden", "1")
						rawBody, _ = io.ReadAll(req.Body)
						_, _ = fmt.Fprintf(rw, "Override")
						return true, nil
					},
				)

				r.ServeHTTP(res, req)
				So(res.Code, ShouldEqual, http.StatusOK)
				So(res.Result().Header, ShouldResemble, http.Header{
					"Content-Type": {"text/plain; charset=utf-8"},
					"Overridden":   {"1"},
				})
				So(rpcReq, ShouldResembleProto, &HelloRequest{Name: "Lucy"})
				So(res.Body.String(), ShouldEqual, "Override")
				So(decodeRequest(rawBody), ShouldResembleProto, &HelloRequest{Name: "Lucy"})
			})

			Convey("Override callback: malformed request, pass through", func() {
				req.Body = io.NopCloser(bytes.NewBufferString("not a proto"))

				var callbackErr error
				server.RegisterOverride("prpc.Greeter", "SayHello",
					func(rw http.ResponseWriter, req *http.Request, body func(msg proto.Message) error) (bool, error) {
						callbackErr = body(&HelloRequest{})
						return false, nil // let the request be handled by the pRPC server
					},
				)

				r.ServeHTTP(res, req)
				So(callbackErr, ShouldErrLike, "could not decode body")
				So(res.Code, ShouldEqual, http.StatusBadRequest)
				So(res.Result().Header, ShouldResemble, http.Header{
					"Content-Type":              {"text/plain"},
					"Date":                      nil,
					"X-Content-Type-Options":    {"nosniff"},
					"X-Prpc-Grpc-Code":          {strCode(codes.InvalidArgument)},
					"X-Prpc-Status-Details-Bin": []string{},
				})
				So(res.Body.String(), ShouldStartWith, "could not decode body")
			})

			Convey("Override callback: malformed request, override", func() {
				req.Body = io.NopCloser(bytes.NewBufferString("not a proto"))

				var callbackErr error
				var rawBody []byte
				var rawBodyErr error
				server.RegisterOverride("prpc.Greeter", "SayHello",
					func(rw http.ResponseWriter, req *http.Request, body func(msg proto.Message) error) (bool, error) {
						callbackErr = body(&HelloRequest{})
						rawBody, rawBodyErr = io.ReadAll(req.Body)
						rw.Header().Set("Overridden", "1")
						_, _ = fmt.Fprintf(rw, "Override")
						return true, nil
					},
				)

				r.ServeHTTP(res, req)
				So(callbackErr, ShouldErrLike, "could not decode body")
				So(string(rawBody), ShouldEqual, "not a proto")
				So(rawBodyErr, ShouldBeNil)
				So(res.Code, ShouldEqual, http.StatusOK)
				So(res.Result().Header, ShouldResemble, http.Header{
					"Content-Type": {"text/plain; charset=utf-8"},
					"Overridden":   {"1"},
				})
				So(res.Body.String(), ShouldEqual, "Override")
			})

			Convey("Override callback: IO error when peeking, pass through", func() {
				req.Body = io.NopCloser(io.MultiReader(
					bytes.NewBufferString(`name: "Zzz"`),
					&erroringReader{errors.New("BOOM")},
				))

				var callbackErr error
				server.RegisterOverride("prpc.Greeter", "SayHello",
					func(rw http.ResponseWriter, req *http.Request, body func(msg proto.Message) error) (bool, error) {
						callbackErr = body(&HelloRequest{})
						return false, nil // let the request be handled by the pRPC server
					},
				)

				r.ServeHTTP(res, req)
				So(callbackErr, ShouldErrLike, "BOOM")
				So(res.Code, ShouldEqual, http.StatusBadRequest)
				So(res.Result().Header, ShouldResemble, http.Header{
					"Content-Type":              {"text/plain"},
					"Date":                      nil,
					"X-Content-Type-Options":    {"nosniff"},
					"X-Prpc-Grpc-Code":          {strCode(codes.InvalidArgument)},
					"X-Prpc-Status-Details-Bin": []string{},
				})
				So(res.Body.String(), ShouldStartWith, "reading the request: BOOM")
			})

			Convey("Override callback: IO error when peeking, override", func() {
				req.Body = io.NopCloser(io.MultiReader(
					bytes.NewBufferString(`name: "Zzz"`),
					&erroringReader{errors.New("BOOM")},
				))

				var callbackErr error
				var rawBody []byte
				var rawBodyErr error
				server.RegisterOverride("prpc.Greeter", "SayHello",
					func(rw http.ResponseWriter, req *http.Request, body func(msg proto.Message) error) (bool, error) {
						callbackErr = body(&HelloRequest{})
						rawBody, rawBodyErr = io.ReadAll(req.Body)
						rw.Header().Set("Overridden", "1")
						_, _ = fmt.Fprintf(rw, "Override")
						return true, nil
					},
				)

				r.ServeHTTP(res, req)
				So(callbackErr, ShouldErrLike, "BOOM")
				So(string(rawBody), ShouldEqual, `name: "Zzz"`)
				So(rawBodyErr, ShouldErrLike, "BOOM")
				So(res.Code, ShouldEqual, http.StatusOK)
				So(res.Result().Header, ShouldResemble, http.Header{
					"Content-Type": {"text/plain; charset=utf-8"},
					"Overridden":   {"1"},
				})
				So(res.Body.String(), ShouldEqual, "Override")
			})

			Convey("Override callback: request too big", func() {
				req.Body = io.NopCloser(bytes.NewReader(make([]byte, server.MaxRequestSize+1)))

				var callbackErr error
				server.RegisterOverride("prpc.Greeter", "SayHello",
					func(rw http.ResponseWriter, req *http.Request, body func(msg proto.Message) error) (bool, error) {
						callbackErr = body(&HelloRequest{})
						return false, nil // let the request be handled by the pRPC server
					},
				)

				r.ServeHTTP(res, req)
				So(callbackErr, ShouldErrLike, "request body too large")
				So(res.Code, ShouldEqual, http.StatusServiceUnavailable)
				So(res.Result().Header, ShouldResemble, http.Header{
					"Content-Type":              {"text/plain"},
					"Date":                      nil,
					"X-Content-Type-Options":    {"nosniff"},
					"X-Prpc-Grpc-Code":          {strCode(codes.Unavailable)},
					"X-Prpc-Status-Details-Bin": []string{},
				})
				So(res.Body.String(), ShouldStartWith, "reading the request: the request size exceeds the server limit")
			})
		})
	})
}

type erroringReader struct {
	err error
}

func (r *erroringReader) Read(p []byte) (n int, err error) {
	return 0, r.err
}
