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
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/server/router"

	"github.com/golang/protobuf/proto"
	. "github.com/smartystreets/goconvey/convey"
)

type greeterService struct {
	headerMD   metadata.MD
	errDetails []proto.Message
}

func (s *greeterService) SayHello(c context.Context, req *HelloRequest) (*HelloReply, error) {
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
		SetHeader(c, s.headerMD)
	}

	return &HelloReply{
		Message: "Hello " + req.Name,
	}, nil
}

type calcService struct{}

func (s *calcService) Multiply(c context.Context, req *MultiplyRequest) (*MultiplyResponse, error) {
	return &MultiplyResponse{
		Z: req.X & req.Y,
	}, nil
}

func TestServer(t *testing.T) {
	t.Parallel()

	Convey("Greeter service", t, func() {
		server := Server{Authenticator: NoAuthentication}

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
					ctx.Context = c
					next(ctx)
				},
			))
			res := httptest.NewRecorder()
			hiMsg := bytes.NewBufferString(`name: "Lucy"`)
			req, err := http.NewRequest("POST", "/prpc/prpc.Greeter/SayHello", hiMsg)
			So(err, ShouldBeNil)
			req.Header.Set("Content-Type", mtPRPCText)

			invalidArgument := strconv.Itoa(int(codes.InvalidArgument))
			unimplemented := strconv.Itoa(int(codes.Unimplemented))

			Convey("Works", func() {
				req.Header.Set("Accept", mtPRPCText)
				r.ServeHTTP(res, req)
				So(res.Code, ShouldEqual, http.StatusOK)
				So(res.Header().Get(HeaderGRPCCode), ShouldEqual, "0")
				So(res.Header().Get("X-Content-Type-Options"), ShouldEqual, "nosniff")
				So(res.Body.String(), ShouldEqual, "message: \"Hello Lucy\"\n")
			})

			Convey("Header Metadata", func() {
				greeterSvc.headerMD = metadata.Pairs("a", "1", "b", "2")
				r.ServeHTTP(res, req)
				So(res.Code, ShouldEqual, http.StatusOK)
				So(res.Header()["A"], ShouldResemble, []string{"1"})
				So(res.Header()["B"], ShouldResemble, []string{"2"})
			})

			Convey("Status details", func() {
				greeterSvc.errDetails = []proto.Message{&errdetails.DebugInfo{Detail: "x"}}
				r.ServeHTTP(res, req)
				So(res.Header()[HeaderStatusDetail], ShouldResemble, []string{
					"Cih0eXBlLmdvb2dsZWFwaXMuY29tL2dvb2dsZS5ycGMuRGVidWdJbmZvEgMSAXg=",
				})
			})

			Convey("Invalid Accept header", func() {
				req.Header.Set("Accept", "blah")
				r.ServeHTTP(res, req)
				So(res.Code, ShouldEqual, http.StatusNotAcceptable)
				So(res.Header().Get(HeaderGRPCCode), ShouldEqual, invalidArgument)
			})

			Convey("Invalid header", func() {
				req.Header.Set("X-Bin", "zzz")
				r.ServeHTTP(res, req)
				So(res.Code, ShouldEqual, http.StatusBadRequest)
				So(res.Header().Get(HeaderGRPCCode), ShouldEqual, invalidArgument)
			})

			Convey("Malformed request message", func() {
				hiMsg.WriteString("\nblah")
				r.ServeHTTP(res, req)
				So(res.Code, ShouldEqual, http.StatusBadRequest)
				So(res.Header().Get(HeaderGRPCCode), ShouldEqual, invalidArgument)
				So(res.Header().Get("X-Content-Type-Options"), ShouldEqual, "nosniff")
			})

			Convey("Invalid request message", func() {
				hiMsg.Reset()
				r.ServeHTTP(res, req)
				So(res.Code, ShouldEqual, http.StatusBadRequest)
				So(res.Header().Get(HeaderGRPCCode), ShouldEqual, invalidArgument)
				So(res.Body.String(), ShouldEqual, "Name unspecified\n")
			})

			Convey("no such service", func() {
				req.URL.Path = "/prpc/xxx/SayHello"
				r.ServeHTTP(res, req)
				So(res.Code, ShouldEqual, http.StatusNotImplemented)
				So(res.Header().Get(HeaderGRPCCode), ShouldEqual, unimplemented)
			})

			Convey("no such method", func() {
				req.URL.Path = "/prpc/Greeter/xxx"
				r.ServeHTTP(res, req)
				So(res.Code, ShouldEqual, http.StatusNotImplemented)
				So(res.Header().Get(HeaderGRPCCode), ShouldEqual, unimplemented)
			})

			Convey(`When access control is enabled without credentials`, func() {
				server.AccessControl = func(c context.Context, origin string) AccessControlDecision {
					return AccessControlDecision{
						AllowCrossOriginRequests: true,
						AllowCredentials:         false,
					}
				}
				req.Header.Add("Origin", "http://example.com")

				r.ServeHTTP(res, req)
				So(res.Code, ShouldEqual, http.StatusOK)
				So(res.Header().Get(HeaderGRPCCode), ShouldEqual, "0")
				So(res.Header().Get("Access-Control-Allow-Origin"), ShouldEqual, "http://example.com")
				So(res.Header().Get("Access-Control-Allow-Credentials"), ShouldEqual, "")
				So(res.Header().Get("Access-Control-Expose-Headers"), ShouldEqual, HeaderGRPCCode)
			})

			Convey(`When access control is enabled for "http://example.com"`, func() {
				decision := AccessControlDecision{
					AllowCrossOriginRequests: true,
					AllowCredentials:         true,
				}
				server.AccessControl = func(c context.Context, origin string) AccessControlDecision {
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
						So(res.Header().Get("Access-Control-Allow-Origin"), ShouldEqual, "http://example.com")
						So(res.Header().Get("Access-Control-Allow-Credentials"), ShouldEqual, "true")
						So(res.Header().Get("Access-Control-Allow-Headers"), ShouldEqual, "Origin, Content-Type, Accept, Authorization")
						So(res.Header().Get("Access-Control-Allow-Methods"), ShouldEqual, "OPTIONS, POST")
						So(res.Header().Get("Access-Control-Max-Age"), ShouldEqual, "600")
					})

					Convey(`Will not supply access-* headers to "http://foo.bar"`, func() {
						req.Header.Add("Origin", "http://foo.bar")

						r.ServeHTTP(res, req)
						So(res.Code, ShouldEqual, http.StatusOK)
						So(res.Header().Get("Access-Control-Allow-Origin"), ShouldEqual, "")
					})
				})

				Convey(`When sending a POST request`, func() {
					Convey(`Will supply Access-* headers to "http://example.com"`, func() {
						req.Header.Add("Origin", "http://example.com")

						r.ServeHTTP(res, req)
						So(res.Code, ShouldEqual, http.StatusOK)
						So(res.Header().Get(HeaderGRPCCode), ShouldEqual, "0")
						So(res.Header().Get("Access-Control-Allow-Origin"), ShouldEqual, "http://example.com")
						So(res.Header().Get("Access-Control-Allow-Credentials"), ShouldEqual, "true")
						So(res.Header().Get("Access-Control-Expose-Headers"), ShouldEqual, HeaderGRPCCode)
					})

					Convey(`Will not supply access-* headers to "http://foo.bar"`, func() {
						req.Header.Add("Origin", "http://foo.bar")

						r.ServeHTTP(res, req)
						So(res.Code, ShouldEqual, http.StatusOK)
						So(res.Header().Get(HeaderGRPCCode), ShouldEqual, "0")
						So(res.Header().Get("Access-Control-Allow-Origin"), ShouldEqual, "")
					})
				})

				Convey(`Using custom AllowHeaders`, func() {
					decision.AllowHeaders = []string{"Booboo", "bobo"}

					req.Method = "OPTIONS"
					req.Header.Add("Origin", "http://example.com")

					r.ServeHTTP(res, req)
					So(res.Code, ShouldEqual, http.StatusOK)
					So(res.Header().Get("Access-Control-Allow-Origin"), ShouldEqual, "http://example.com")
					So(res.Header().Get("Access-Control-Allow-Credentials"), ShouldEqual, "true")
					So(res.Header().Get("Access-Control-Allow-Headers"), ShouldEqual, "Booboo, bobo, Origin, Content-Type, Accept, Authorization")
					So(res.Header().Get("Access-Control-Allow-Methods"), ShouldEqual, "OPTIONS, POST")
					So(res.Header().Get("Access-Control-Max-Age"), ShouldEqual, "600")
				})
			})
		})
	})
}
