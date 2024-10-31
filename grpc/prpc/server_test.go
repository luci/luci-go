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

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/router"

	"go.chromium.org/luci/grpc/prpc/internal/testpb"
)

type greeterService struct {
	testpb.UnimplementedGreeterServer

	headerMD   metadata.MD
	errDetails *errdetails.DebugInfo
}

func (s *greeterService) SayHello(ctx context.Context, req *testpb.HelloRequest) (*testpb.HelloReply, error) {
	if req.Name == "" {
		return nil, status.Errorf(codes.InvalidArgument, "Name unspecified")
	}

	if s.errDetails != nil {
		st, err := status.New(codes.Unknown, "").WithDetails(s.errDetails)
		if err == nil {
			err = st.Err()
		}
		return nil, err
	}

	if s.headerMD != nil {
		SetHeader(ctx, s.headerMD)
	}

	return &testpb.HelloReply{
		Message: "Hello " + req.Name,
	}, nil
}

type calcService struct {
	testpb.UnimplementedCalcServer
}

func (s *calcService) Multiply(ctx context.Context, req *testpb.MultiplyRequest) (*testpb.MultiplyResponse, error) {
	return &testpb.MultiplyResponse{
		Z: req.X & req.Y,
	}, nil
}

func decodeRequest(body []byte) *testpb.HelloRequest {
	msg := &testpb.HelloRequest{}
	if err := prototext.Unmarshal(body, msg); err != nil {
		panic(err)
	}
	return msg
}

func decodeReply(body []byte) *testpb.HelloReply {
	msg := &testpb.HelloReply{}
	if err := prototext.Unmarshal(body, msg); err != nil {
		panic(err)
	}
	return msg
}

func TestServer(t *testing.T) {
	t.Parallel()

	ftt.Run("Greeter service", t, func(t *ftt.Test) {
		server := Server{MaxRequestSize: 100}

		greeterSvc := &greeterService{}
		testpb.RegisterGreeterServer(&server, greeterSvc)

		t.Run("Register Calc service", func(t *ftt.Test) {
			testpb.RegisterCalcServer(&server, &calcService{})
			assert.Loosely(t, server.ServiceNames(), should.Resemble([]string{
				"prpc.Calc",
				"prpc.Greeter",
			}))
		})

		t.Run("Handlers", func(t *ftt.Test) {
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
			assert.Loosely(t, err, should.BeNil)
			req.Header.Set("Content-Type", mtPRPCText)

			strCode := func(c codes.Code) string {
				return strconv.Itoa(int(c))
			}

			t.Run("Works", func(t *ftt.Test) {
				req.Header.Set("Accept", mtPRPCText)
				r.ServeHTTP(res, req)
				assert.Loosely(t, res.Code, should.Equal(http.StatusOK))
				assert.Loosely(t, res.Result().Header, should.Resemble(http.Header{
					"Content-Type":           {"application/prpc; encoding=text"},
					"Date":                   nil,
					"X-Content-Type-Options": {"nosniff"},
					"X-Prpc-Grpc-Code":       {strCode(codes.OK)},
				}))
				assert.Loosely(t, decodeReply(res.Body.Bytes()), should.Resemble(&testpb.HelloReply{Message: "Hello Lucy"}))
			})

			t.Run("Header Metadata", func(t *ftt.Test) {
				greeterSvc.headerMD = metadata.Pairs("a", "1", "b", "2", "date", "2112")
				r.ServeHTTP(res, req)
				assert.Loosely(t, res.Code, should.Equal(http.StatusOK))
				assert.Loosely(t, res.Result().Header, should.Resemble(http.Header{
					"A":                      {"1"},
					"B":                      {"2"},
					"Content-Type":           {"application/prpc; encoding=binary"},
					"Date":                   {"2112"},
					"X-Content-Type-Options": {"nosniff"},
					"X-Prpc-Grpc-Code":       {strCode(codes.OK)},
				}))
			})

			t.Run("Status details", func(t *ftt.Test) {
				greeterSvc.errDetails = &errdetails.DebugInfo{Detail: "x"}
				r.ServeHTTP(res, req)
				assert.Loosely(t, res.Result().Header, should.Resemble(http.Header{
					"Content-Type":              {"text/plain; charset=utf-8"},
					"Date":                      nil,
					"X-Content-Type-Options":    {"nosniff"},
					"X-Prpc-Grpc-Code":          {strCode(codes.Unknown)},
					"X-Prpc-Status-Details-Bin": {"Cih0eXBlLmdvb2dsZWFwaXMuY29tL2dvb2dsZS5ycGMuRGVidWdJbmZvEgMSAXg="},
				}))
			})

			t.Run("Invalid Accept header", func(t *ftt.Test) {
				req.Header.Set("Accept", "blah")
				r.ServeHTTP(res, req)
				assert.Loosely(t, res.Code, should.Equal(http.StatusNotAcceptable))
				assert.Loosely(t, res.Result().Header, should.Resemble(http.Header{
					"Content-Type":           {"text/plain; charset=utf-8"},
					"Date":                   nil,
					"X-Content-Type-Options": {"nosniff"},
					"X-Prpc-Grpc-Code":       {strCode(codes.InvalidArgument)},
				}))
			})

			t.Run("Invalid max response size header", func(t *ftt.Test) {
				for _, bad := range []string{"not-an-int", "0", "-1", "   123"} {
					req.Header.Set(HeaderMaxResponseSize, bad)
					r.ServeHTTP(res, req)
					assert.Loosely(t, res.Code, should.Equal(http.StatusBadRequest))
					assert.Loosely(t, res.Result().Header, should.Resemble(http.Header{
						"Content-Type":           {"text/plain; charset=utf-8"},
						"Date":                   nil,
						"X-Content-Type-Options": {"nosniff"},
						"X-Prpc-Grpc-Code":       {strCode(codes.InvalidArgument)},
					}))
				}
			})

			t.Run("Invalid header", func(t *ftt.Test) {
				req.Header.Set("X-Bin", "zzz")
				r.ServeHTTP(res, req)
				assert.Loosely(t, res.Code, should.Equal(http.StatusBadRequest))
				assert.Loosely(t, res.Result().Header, should.Resemble(http.Header{
					"Content-Type":           {"text/plain; charset=utf-8"},
					"Date":                   nil,
					"X-Content-Type-Options": {"nosniff"},
					"X-Prpc-Grpc-Code":       {strCode(codes.InvalidArgument)},
				}))
			})

			t.Run("Malformed request message", func(t *ftt.Test) {
				hiMsg.WriteString("\nblah")
				r.ServeHTTP(res, req)
				assert.Loosely(t, res.Code, should.Equal(http.StatusBadRequest))
				assert.Loosely(t, res.Result().Header, should.Resemble(http.Header{
					"Content-Type":           {"text/plain; charset=utf-8"},
					"Date":                   nil,
					"X-Content-Type-Options": {"nosniff"},
					"X-Prpc-Grpc-Code":       {strCode(codes.InvalidArgument)},
				}))
			})

			t.Run("Invalid request message", func(t *ftt.Test) {
				hiMsg.Reset()
				r.ServeHTTP(res, req)
				assert.Loosely(t, res.Code, should.Equal(http.StatusBadRequest))
				assert.Loosely(t, res.Result().Header, should.Resemble(http.Header{
					"Content-Type":           {"text/plain; charset=utf-8"},
					"Date":                   nil,
					"X-Content-Type-Options": {"nosniff"},
					"X-Prpc-Grpc-Code":       {strCode(codes.InvalidArgument)},
				}))
				assert.Loosely(t, res.Body.String(), should.Equal("Name unspecified\n"))
			})

			t.Run("no such service", func(t *ftt.Test) {
				req.URL.Path = "/prpc/xxx/SayHello"
				r.ServeHTTP(res, req)
				assert.Loosely(t, res.Code, should.Equal(http.StatusNotImplemented))
				assert.Loosely(t, res.Result().Header, should.Resemble(http.Header{
					"Content-Type":           {"text/plain; charset=utf-8"},
					"Date":                   nil,
					"X-Content-Type-Options": {"nosniff"},
					"X-Prpc-Grpc-Code":       {strCode(codes.Unimplemented)},
				}))
			})

			t.Run("no such method", func(t *ftt.Test) {
				req.URL.Path = "/prpc/Greeter/xxx"
				r.ServeHTTP(res, req)
				assert.Loosely(t, res.Code, should.Equal(http.StatusNotImplemented))
				assert.Loosely(t, res.Result().Header, should.Resemble(http.Header{
					"Content-Type":           {"text/plain; charset=utf-8"},
					"Date":                   nil,
					"X-Content-Type-Options": {"nosniff"},
					"X-Prpc-Grpc-Code":       {strCode(codes.Unimplemented)},
				}))
			})

			t.Run(`When access control is enabled without credentials`, func(t *ftt.Test) {
				server.AccessControl = func(ctx context.Context, origin string) AccessControlDecision {
					return AccessControlDecision{
						AllowCrossOriginRequests: true,
						AllowCredentials:         false,
					}
				}
				req.Header.Add("Origin", "http://example.com")

				r.ServeHTTP(res, req)
				assert.Loosely(t, res.Code, should.Equal(http.StatusOK))
				assert.Loosely(t, res.Result().Header, should.Resemble(http.Header{
					"Access-Control-Allow-Origin":   {"http://example.com"},
					"Access-Control-Expose-Headers": {"X-Prpc-Grpc-Code, X-Prpc-Status-Details-Bin"},
					"Content-Type":                  {"application/prpc; encoding=binary"},
					"Date":                          nil,
					"Vary":                          {"Origin"},
					"X-Content-Type-Options":        {"nosniff"},
					"X-Prpc-Grpc-Code":              {strCode(codes.OK)},
				}))
			})

			t.Run(`When access control is enabled for "http://example.com"`, func(t *ftt.Test) {
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

				t.Run(`When sending an OPTIONS request`, func(t *ftt.Test) {
					req.Method = "OPTIONS"

					t.Run(`Will supply Access-* headers to "http://example.com"`, func(t *ftt.Test) {
						req.Header.Add("Origin", "http://example.com")

						r.ServeHTTP(res, req)
						assert.Loosely(t, res.Code, should.Equal(http.StatusOK))
						assert.Loosely(t, res.Result().Header, should.Resemble(http.Header{
							"Access-Control-Allow-Credentials": {"true"},
							"Access-Control-Allow-Headers":     {"Origin, Content-Type, Accept, Authorization, X-Prpc-Grpc-Timeout, X-Prpc-Max-Response-Size"},
							"Access-Control-Allow-Methods":     {"OPTIONS, POST"},
							"Access-Control-Allow-Origin":      {"http://example.com"},
							"Access-Control-Max-Age":           {"600"},
							"Vary":                             {"Origin"},
						}))
					})

					t.Run(`Will not supply access-* headers to "http://foo.bar"`, func(t *ftt.Test) {
						req.Header.Add("Origin", "http://foo.bar")

						r.ServeHTTP(res, req)
						assert.Loosely(t, res.Code, should.Equal(http.StatusOK))
						assert.Loosely(t, res.Result().Header, should.Resemble(http.Header{}))
					})
				})

				t.Run(`When sending a POST request`, func(t *ftt.Test) {
					t.Run(`Will supply Access-* headers to "http://example.com"`, func(t *ftt.Test) {
						req.Header.Add("Origin", "http://example.com")

						r.ServeHTTP(res, req)
						assert.Loosely(t, res.Code, should.Equal(http.StatusOK))
						assert.Loosely(t, res.Result().Header, should.Resemble(http.Header{
							"Access-Control-Allow-Credentials": {"true"},
							"Access-Control-Allow-Origin":      {"http://example.com"},
							"Access-Control-Expose-Headers":    {"X-Prpc-Grpc-Code, X-Prpc-Status-Details-Bin"},
							"Content-Type":                     {"application/prpc; encoding=binary"},
							"Date":                             nil,
							"Vary":                             {"Origin"},
							"X-Content-Type-Options":           {"nosniff"},
							"X-Prpc-Grpc-Code":                 {strCode(codes.OK)},
						}))
					})

					t.Run(`Will not supply access-* headers to "http://foo.bar"`, func(t *ftt.Test) {
						req.Header.Add("Origin", "http://foo.bar")

						r.ServeHTTP(res, req)
						assert.Loosely(t, res.Code, should.Equal(http.StatusOK))
						assert.Loosely(t, res.Result().Header, should.Resemble(http.Header{
							"Content-Type":           {"application/prpc; encoding=binary"},
							"Date":                   nil,
							"X-Content-Type-Options": {"nosniff"},
							"X-Prpc-Grpc-Code":       {strCode(codes.OK)},
						}))
					})
				})

				t.Run(`Using custom AllowHeaders`, func(t *ftt.Test) {
					decision.AllowHeaders = []string{"Booboo", "bobo"}

					req.Method = "OPTIONS"
					req.Header.Add("Origin", "http://example.com")

					r.ServeHTTP(res, req)
					assert.Loosely(t, res.Code, should.Equal(http.StatusOK))
					assert.Loosely(t, res.Result().Header, should.Resemble(http.Header{
						"Access-Control-Allow-Credentials": {"true"},
						"Access-Control-Allow-Headers":     {"Booboo, bobo, Origin, Content-Type, Accept, Authorization, X-Prpc-Grpc-Timeout, X-Prpc-Max-Response-Size"},
						"Access-Control-Allow-Methods":     {"OPTIONS, POST"},
						"Access-Control-Allow-Origin":      {"http://example.com"},
						"Access-Control-Max-Age":           {"600"},
						"Vary":                             {"Origin"},
					}))
				})
			})

			t.Run("Override callback: pass through", func(t *ftt.Test) {
				req.Header.Set("Accept", mtPRPCText)

				called := false
				server.RegisterOverride("prpc.Greeter", "SayHello",
					func(rw http.ResponseWriter, req *http.Request, body func(msg proto.Message) error) (bool, error) {
						called = true
						return false, nil
					},
				)

				r.ServeHTTP(res, req)
				assert.Loosely(t, res.Code, should.Equal(http.StatusOK))
				assert.Loosely(t, res.Result().Header, should.Resemble(http.Header{
					"Content-Type":           {"application/prpc; encoding=text"},
					"Date":                   nil,
					"X-Content-Type-Options": {"nosniff"},
					"X-Prpc-Grpc-Code":       {strCode(codes.OK)},
				}))
				assert.Loosely(t, decodeReply(res.Body.Bytes()), should.Resemble(&testpb.HelloReply{Message: "Hello Lucy"}))
				assert.Loosely(t, called, should.BeTrue)
			})

			t.Run("Override callback: override", func(t *ftt.Test) {
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
				assert.Loosely(t, res.Code, should.Equal(http.StatusOK))
				assert.Loosely(t, res.Result().Header, should.Resemble(http.Header{
					"Content-Type": {"text/plain; charset=utf-8"},
					"Overridden":   {"1"},
				}))
				assert.Loosely(t, res.Body.String(), should.Equal("Override"))
				assert.Loosely(t, decodeRequest(rawBody), should.Resemble(&testpb.HelloRequest{Name: "Lucy"}))
			})

			t.Run("Override callback: error", func(t *ftt.Test) {
				req.Header.Set("Accept", mtPRPCText)

				server.RegisterOverride("prpc.Greeter", "SayHello",
					func(rw http.ResponseWriter, req *http.Request, body func(msg proto.Message) error) (bool, error) {
						_, _ = io.ReadAll(req.Body)
						return false, errors.New("BOOM")
					},
				)

				r.ServeHTTP(res, req)
				assert.Loosely(t, res.Code, should.Equal(http.StatusBadRequest))
				assert.Loosely(t, res.Result().Header, should.Resemble(http.Header{
					"Content-Type":           {"text/plain; charset=utf-8"},
					"Date":                   nil,
					"X-Content-Type-Options": {"nosniff"},
					"X-Prpc-Grpc-Code":       {strCode(codes.InvalidArgument)},
				}))
				assert.Loosely(t, res.Body.String(), should.Equal("the override check: BOOM\n"))
			})

			t.Run("Override callback: peek, pass through", func(t *ftt.Test) {
				req.Header.Set("Accept", mtPRPCText)

				var rpcReq *testpb.HelloRequest
				server.RegisterOverride("prpc.Greeter", "SayHello",
					func(rw http.ResponseWriter, req *http.Request, body func(msg proto.Message) error) (bool, error) {
						r := &testpb.HelloRequest{}
						if err := body(r); err != nil {
							return false, err
						}
						rpcReq = r
						return false, nil
					},
				)

				r.ServeHTTP(res, req)
				assert.Loosely(t, res.Code, should.Equal(http.StatusOK))
				assert.Loosely(t, res.Result().Header, should.Resemble(http.Header{
					"Content-Type":           {"application/prpc; encoding=text"},
					"Date":                   nil,
					"X-Content-Type-Options": {"nosniff"},
					"X-Prpc-Grpc-Code":       {strCode(codes.OK)},
				}))
				assert.Loosely(t, decodeReply(res.Body.Bytes()), should.Resemble(&testpb.HelloReply{Message: "Hello Lucy"}))
				assert.Loosely(t, rpcReq, should.Resemble(&testpb.HelloRequest{Name: "Lucy"}))
			})

			t.Run("Override callback: peek, override", func(t *ftt.Test) {
				req.Header.Set("Accept", mtPRPCText)

				var rpcReq *testpb.HelloRequest
				var rawBody []byte
				server.RegisterOverride("prpc.Greeter", "SayHello",
					func(rw http.ResponseWriter, req *http.Request, body func(msg proto.Message) error) (bool, error) {
						r := &testpb.HelloRequest{}
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
				assert.Loosely(t, res.Code, should.Equal(http.StatusOK))
				assert.Loosely(t, res.Result().Header, should.Resemble(http.Header{
					"Content-Type": {"text/plain; charset=utf-8"},
					"Overridden":   {"1"},
				}))
				assert.Loosely(t, rpcReq, should.Resemble(&testpb.HelloRequest{Name: "Lucy"}))
				assert.Loosely(t, res.Body.String(), should.Equal("Override"))
				assert.Loosely(t, decodeRequest(rawBody), should.Resemble(&testpb.HelloRequest{Name: "Lucy"}))
			})

			t.Run("Override callback: malformed request, pass through", func(t *ftt.Test) {
				req.Body = io.NopCloser(bytes.NewBufferString("not a proto"))

				var callbackErr error
				server.RegisterOverride("prpc.Greeter", "SayHello",
					func(rw http.ResponseWriter, req *http.Request, body func(msg proto.Message) error) (bool, error) {
						callbackErr = body(&testpb.HelloRequest{})
						return false, nil // let the request be handled by the pRPC server
					},
				)

				r.ServeHTTP(res, req)
				assert.Loosely(t, callbackErr, should.ErrLike("could not decode body"))
				assert.Loosely(t, res.Code, should.Equal(http.StatusBadRequest))
				assert.Loosely(t, res.Result().Header, should.Resemble(http.Header{
					"Content-Type":           {"text/plain; charset=utf-8"},
					"Date":                   nil,
					"X-Content-Type-Options": {"nosniff"},
					"X-Prpc-Grpc-Code":       {strCode(codes.InvalidArgument)},
				}))
				assert.Loosely(t, res.Body.String(), should.HavePrefix("could not decode body"))
			})

			t.Run("Override callback: malformed request, override", func(t *ftt.Test) {
				req.Body = io.NopCloser(bytes.NewBufferString("not a proto"))

				var callbackErr error
				var rawBody []byte
				var rawBodyErr error
				server.RegisterOverride("prpc.Greeter", "SayHello",
					func(rw http.ResponseWriter, req *http.Request, body func(msg proto.Message) error) (bool, error) {
						callbackErr = body(&testpb.HelloRequest{})
						rawBody, rawBodyErr = io.ReadAll(req.Body)
						rw.Header().Set("Overridden", "1")
						_, _ = fmt.Fprintf(rw, "Override")
						return true, nil
					},
				)

				r.ServeHTTP(res, req)
				assert.Loosely(t, callbackErr, should.ErrLike("could not decode body"))
				assert.Loosely(t, string(rawBody), should.Equal("not a proto"))
				assert.Loosely(t, rawBodyErr, should.BeNil)
				assert.Loosely(t, res.Code, should.Equal(http.StatusOK))
				assert.Loosely(t, res.Result().Header, should.Resemble(http.Header{
					"Content-Type": {"text/plain; charset=utf-8"},
					"Overridden":   {"1"},
				}))
				assert.Loosely(t, res.Body.String(), should.Equal("Override"))
			})

			t.Run("Override callback: IO error when peeking, pass through", func(t *ftt.Test) {
				req.Body = io.NopCloser(io.MultiReader(
					bytes.NewBufferString(`name: "Zzz"`),
					&erroringReader{errors.New("BOOM")},
				))

				var callbackErr error
				server.RegisterOverride("prpc.Greeter", "SayHello",
					func(rw http.ResponseWriter, req *http.Request, body func(msg proto.Message) error) (bool, error) {
						callbackErr = body(&testpb.HelloRequest{})
						return false, nil // let the request be handled by the pRPC server
					},
				)

				r.ServeHTTP(res, req)
				assert.Loosely(t, callbackErr, should.ErrLike("BOOM"))
				assert.Loosely(t, res.Code, should.Equal(http.StatusBadRequest))
				assert.Loosely(t, res.Result().Header, should.Resemble(http.Header{
					"Content-Type":           {"text/plain; charset=utf-8"},
					"Date":                   nil,
					"X-Content-Type-Options": {"nosniff"},
					"X-Prpc-Grpc-Code":       {strCode(codes.InvalidArgument)},
				}))
				assert.Loosely(t, res.Body.String(), should.HavePrefix("reading the request: BOOM"))
			})

			t.Run("Override callback: IO error when peeking, override", func(t *ftt.Test) {
				req.Body = io.NopCloser(io.MultiReader(
					bytes.NewBufferString(`name: "Zzz"`),
					&erroringReader{errors.New("BOOM")},
				))

				var callbackErr error
				var rawBody []byte
				var rawBodyErr error
				server.RegisterOverride("prpc.Greeter", "SayHello",
					func(rw http.ResponseWriter, req *http.Request, body func(msg proto.Message) error) (bool, error) {
						callbackErr = body(&testpb.HelloRequest{})
						rawBody, rawBodyErr = io.ReadAll(req.Body)
						rw.Header().Set("Overridden", "1")
						_, _ = fmt.Fprintf(rw, "Override")
						return true, nil
					},
				)

				r.ServeHTTP(res, req)
				assert.Loosely(t, callbackErr, should.ErrLike("BOOM"))
				assert.Loosely(t, string(rawBody), should.Equal(`name: "Zzz"`))
				assert.Loosely(t, rawBodyErr, should.ErrLike("BOOM"))
				assert.Loosely(t, res.Code, should.Equal(http.StatusOK))
				assert.Loosely(t, res.Result().Header, should.Resemble(http.Header{
					"Content-Type": {"text/plain; charset=utf-8"},
					"Overridden":   {"1"},
				}))
				assert.Loosely(t, res.Body.String(), should.Equal("Override"))
			})

			t.Run("Override callback: request too big", func(t *ftt.Test) {
				req.Body = io.NopCloser(bytes.NewReader(make([]byte, server.MaxRequestSize+1)))

				var callbackErr error
				server.RegisterOverride("prpc.Greeter", "SayHello",
					func(rw http.ResponseWriter, req *http.Request, body func(msg proto.Message) error) (bool, error) {
						callbackErr = body(&testpb.HelloRequest{})
						return false, nil // let the request be handled by the pRPC server
					},
				)

				r.ServeHTTP(res, req)
				assert.Loosely(t, callbackErr, should.ErrLike("request body too large"))
				assert.Loosely(t, res.Code, should.Equal(http.StatusServiceUnavailable))
				assert.Loosely(t, res.Result().Header, should.Resemble(http.Header{
					"Content-Type":           {"text/plain; charset=utf-8"},
					"Date":                   nil,
					"X-Content-Type-Options": {"nosniff"},
					"X-Prpc-Grpc-Code":       {strCode(codes.Unavailable)},
				}))
				assert.Loosely(t, res.Body.String(), should.HavePrefix("reading the request: the request size exceeds the server limit"))
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
