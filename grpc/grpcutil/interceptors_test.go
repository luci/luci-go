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

package grpcutil

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	. "github.com/smartystreets/goconvey/convey"
)

func TestChainUnaryServerInterceptors(t *testing.T) {
	t.Parallel()

	Convey("With interceptors", t, func() {
		testCtxKey := "testing"
		testInfo := &grpc.UnaryServerInfo{} // constant address for assertions
		testResponse := new(int)            // constant address for assertions
		testError := errors.New("boom")     // constant address for assertions

		calls := []string{}
		record := func(fn string) func() {
			calls = append(calls, "-> "+fn)
			return func() { calls = append(calls, "<- "+fn) }
		}

		callChain := func(intr grpc.UnaryServerInterceptor, h grpc.UnaryHandler) (interface{}, error) {
			return intr(context.Background(), "request", testInfo, h)
		}

		// A "library" of interceptors used below.

		doNothing := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
			defer record("doNothing")()
			return handler(ctx, req)
		}

		populateContext := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
			defer record("populateContext")()
			return handler(context.WithValue(ctx, &testCtxKey, "value"), req)
		}

		checkContext := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
			defer record("checkContext")()
			So(ctx.Value(&testCtxKey), ShouldEqual, "value")
			return handler(ctx, req)
		}

		modifyReq := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
			defer record("modifyReq")()
			return handler(ctx, "modified request")
		}

		checkReq := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
			defer record("checkReq")()
			So(req.(string), ShouldEqual, "modified request")
			return handler(ctx, req)
		}

		checkErr := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
			defer record("checkErr")()
			resp, err := handler(ctx, req)
			So(err, ShouldEqual, testError)
			return resp, err
		}

		abortChain := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
			defer record("abortChain")()
			return nil, testError
		}

		successHandler := func(ctx context.Context, req interface{}) (interface{}, error) {
			defer record("successHandler")()
			return testResponse, nil
		}

		errorHandler := func(ctx context.Context, req interface{}) (interface{}, error) {
			defer record("errorHandler")()
			return nil, testError
		}

		Convey("Noop chain", func() {
			resp, err := callChain(ChainUnaryServerInterceptors(nil, nil), successHandler)
			So(err, ShouldBeNil)
			So(resp, ShouldEqual, testResponse)
			So(calls, ShouldResemble, []string{
				"-> successHandler",
				"<- successHandler",
			})
		})

		Convey("One link chain", func() {
			resp, err := callChain(ChainUnaryServerInterceptors(doNothing), successHandler)
			So(err, ShouldBeNil)
			So(resp, ShouldEqual, testResponse)
			So(calls, ShouldResemble, []string{
				"-> doNothing",
				"-> successHandler",
				"<- successHandler",
				"<- doNothing",
			})
		})

		Convey("Nils are OK", func() {
			resp, err := callChain(ChainUnaryServerInterceptors(nil, doNothing, nil, nil), successHandler)
			So(err, ShouldBeNil)
			So(resp, ShouldEqual, testResponse)
			So(calls, ShouldResemble, []string{
				"-> doNothing",
				"-> successHandler",
				"<- successHandler",
				"<- doNothing",
			})
		})

		Convey("Changes propagate", func() {
			chain := ChainUnaryServerInterceptors(
				populateContext,
				modifyReq,
				doNothing,
				checkContext,
				checkReq,
			)
			resp, err := callChain(chain, successHandler)
			So(err, ShouldBeNil)
			So(resp, ShouldEqual, testResponse)
			So(calls, ShouldResemble, []string{
				"-> populateContext",
				"-> modifyReq",
				"-> doNothing",
				"-> checkContext",
				"-> checkReq",
				"-> successHandler",
				"<- successHandler",
				"<- checkReq",
				"<- checkContext",
				"<- doNothing",
				"<- modifyReq",
				"<- populateContext",
			})
		})

		Convey("Request error propagates", func() {
			chain := ChainUnaryServerInterceptors(
				doNothing,
				checkErr,
			)
			_, err := callChain(chain, errorHandler)
			So(err, ShouldEqual, testError)
			So(calls, ShouldResemble, []string{
				"-> doNothing",
				"-> checkErr",
				"-> errorHandler",
				"<- errorHandler",
				"<- checkErr",
				"<- doNothing",
			})
		})

		Convey("Interceptor can abort the chain", func() {
			chain := ChainUnaryServerInterceptors(
				doNothing,
				abortChain,
				doNothing,
				doNothing,
				doNothing,
				doNothing,
			)
			_, err := callChain(chain, successHandler)
			So(err, ShouldEqual, testError)
			So(calls, ShouldResemble, []string{
				"-> doNothing",
				"-> abortChain",
				"<- abortChain",
				"<- doNothing",
			})
		})
	})
}

func TestChainStreamServerInterceptors(t *testing.T) {
	t.Parallel()

	// Note: this is 80% copy-pasta of TestChainUnaryServerInterceptors just using
	// different types to match StreamServerInterceptor API.

	Convey("With interceptors", t, func() {
		testCtxKey := "testing"
		testInfo := &grpc.StreamServerInfo{} // constant address for assertions
		testError := errors.New("boom")      // constant address for assertions

		calls := []string{}
		record := func(fn string) func() {
			calls = append(calls, "-> "+fn)
			return func() { calls = append(calls, "<- "+fn) }
		}

		callChain := func(intr grpc.StreamServerInterceptor, h grpc.StreamHandler) error {
			// Note: this will panic horribly when most "real" methods are called, but
			// tests call only Context() and it will be fine.
			phonyStream := &wrappedSS{nil, context.Background()}
			return intr("phony srv", phonyStream, testInfo, h)
		}

		// A "library" of interceptors used below.

		doNothing := func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
			defer record("doNothing")()
			return handler(srv, ss)
		}

		populateContext := func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
			defer record("populateContext")()
			return handler(srv, ModifyServerStreamContext(ss, func(ctx context.Context) context.Context {
				return context.WithValue(ctx, &testCtxKey, "value")
			}))
		}

		checkContext := func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
			defer record("checkContext")()
			So(ss.Context().Value(&testCtxKey), ShouldEqual, "value")
			return handler(srv, ss)
		}

		modifySrv := func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
			defer record("modifySrv")()
			return handler("modified srv", ss)
		}

		checkSrv := func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
			defer record("checkSrv")()
			So(srv.(string), ShouldEqual, "modified srv")
			return handler(srv, ss)
		}

		checkErr := func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
			defer record("checkErr")()
			err := handler(srv, ss)
			So(err, ShouldEqual, testError)
			return err
		}

		abortChain := func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
			defer record("abortChain")()
			return testError
		}

		successHandler := func(srv interface{}, ss grpc.ServerStream) error {
			defer record("successHandler")()
			return nil
		}

		errorHandler := func(srv interface{}, ss grpc.ServerStream) error {
			defer record("errorHandler")()
			return testError
		}

		Convey("Noop chain", func() {
			err := callChain(ChainStreamServerInterceptors(nil, nil), successHandler)
			So(err, ShouldBeNil)
			So(calls, ShouldResemble, []string{
				"-> successHandler",
				"<- successHandler",
			})
		})

		Convey("One link chain", func() {
			err := callChain(ChainStreamServerInterceptors(doNothing), successHandler)
			So(err, ShouldBeNil)
			So(calls, ShouldResemble, []string{
				"-> doNothing",
				"-> successHandler",
				"<- successHandler",
				"<- doNothing",
			})
		})

		Convey("Nils are OK", func() {
			err := callChain(ChainStreamServerInterceptors(nil, doNothing, nil, nil), successHandler)
			So(err, ShouldBeNil)
			So(calls, ShouldResemble, []string{
				"-> doNothing",
				"-> successHandler",
				"<- successHandler",
				"<- doNothing",
			})
		})

		Convey("Changes propagate", func() {
			chain := ChainStreamServerInterceptors(
				populateContext,
				modifySrv,
				doNothing,
				checkContext,
				checkSrv,
			)
			err := callChain(chain, successHandler)
			So(err, ShouldBeNil)
			So(calls, ShouldResemble, []string{
				"-> populateContext",
				"-> modifySrv",
				"-> doNothing",
				"-> checkContext",
				"-> checkSrv",
				"-> successHandler",
				"<- successHandler",
				"<- checkSrv",
				"<- checkContext",
				"<- doNothing",
				"<- modifySrv",
				"<- populateContext",
			})
		})

		Convey("Request error propagates", func() {
			chain := ChainStreamServerInterceptors(
				doNothing,
				checkErr,
			)
			err := callChain(chain, errorHandler)
			So(err, ShouldEqual, testError)
			So(calls, ShouldResemble, []string{
				"-> doNothing",
				"-> checkErr",
				"-> errorHandler",
				"<- errorHandler",
				"<- checkErr",
				"<- doNothing",
			})
		})

		Convey("Interceptor can abort the chain", func() {
			chain := ChainStreamServerInterceptors(
				doNothing,
				abortChain,
				doNothing,
				doNothing,
				doNothing,
				doNothing,
			)
			err := callChain(chain, successHandler)
			So(err, ShouldEqual, testError)
			So(calls, ShouldResemble, []string{
				"-> doNothing",
				"-> abortChain",
				"<- abortChain",
				"<- doNothing",
			})
		})
	})
}

func TestUnifiedServerInterceptor(t *testing.T) {
	t.Parallel()

	type key string // to shut up golint

	unaryInfo := &grpc.UnaryServerInfo{FullMethod: "/svc/method"}
	streamInfo := &grpc.StreamServerInfo{FullMethod: "/svc/method"}

	reqBody := "request"
	resBody := "response"

	rootCtx := context.WithValue(context.Background(), key("x"), "y")
	server := &struct{}{}
	stream := &wrappedSS{nil, rootCtx}

	Convey("Passes requests, modifies the context", t, func() {
		var u UnifiedServerInterceptor = func(ctx context.Context, fullMethod string, handler func(ctx context.Context) error) error {
			So(ctx, ShouldEqual, rootCtx)
			So(fullMethod, ShouldEqual, "/svc/method")
			return handler(context.WithValue(ctx, key("key"), "val"))
		}

		Convey("Unary", func() {
			resp, err := u.Unary()(rootCtx, &reqBody, unaryInfo, func(ctx context.Context, req interface{}) (interface{}, error) {
				So(ctx.Value(key("key")).(string), ShouldEqual, "val")
				So(req, ShouldEqual, &reqBody)
				return &resBody, nil
			})
			So(err, ShouldBeNil)
			So(resp, ShouldEqual, &resBody)
		})

		Convey("Stream", func() {
			err := u.Stream()(server, stream, streamInfo, func(srv interface{}, ss grpc.ServerStream) error {
				So(srv, ShouldEqual, server)
				So(ss.Context().Value(key("key")).(string), ShouldEqual, "val")
				return nil
			})
			So(err, ShouldBeNil)
		})
	})

	Convey("Sees errors", t, func() {
		retErr := status.Errorf(codes.Unknown, "boo")
		var seenErr error

		var u UnifiedServerInterceptor = func(ctx context.Context, fullMethod string, handler func(ctx context.Context) error) error {
			seenErr = handler(ctx)
			return seenErr
		}

		Convey("Unary", func() {
			resp, err := u.Unary()(rootCtx, &reqBody, unaryInfo, func(ctx context.Context, req interface{}) (interface{}, error) {
				return &resBody, retErr
			})
			So(err, ShouldEqual, retErr)
			So(seenErr, ShouldEqual, retErr)
			So(resp, ShouldBeNil)
		})

		Convey("Stream", func() {
			err := u.Stream()(server, stream, streamInfo, func(srv interface{}, ss grpc.ServerStream) error {
				return retErr
			})
			So(err, ShouldEqual, retErr)
			So(seenErr, ShouldEqual, retErr)
		})
	})

	Convey("Can block requests", t, func() {
		retErr := status.Errorf(codes.Unknown, "boo")

		var u UnifiedServerInterceptor = func(ctx context.Context, fullMethod string, handler func(ctx context.Context) error) error {
			return retErr
		}

		Convey("Unary", func() {
			resp, err := u.Unary()(rootCtx, &reqBody, unaryInfo, func(ctx context.Context, req interface{}) (interface{}, error) {
				panic("must not be called")
			})
			So(err, ShouldEqual, retErr)
			So(resp, ShouldBeNil)
		})

		Convey("Stream", func() {
			err := u.Stream()(server, stream, streamInfo, func(srv interface{}, ss grpc.ServerStream) error {
				panic("must not be called")
			})
			So(err, ShouldEqual, retErr)
		})
	})

	Convey("Can override error", t, func() {
		retErr := status.Errorf(codes.Unknown, "boo")

		var u UnifiedServerInterceptor = func(ctx context.Context, fullMethod string, handler func(ctx context.Context) error) error {
			_ = handler(ctx)
			return retErr
		}

		Convey("Unary", func() {
			resp, err := u.Unary()(rootCtx, &reqBody, unaryInfo, func(ctx context.Context, req interface{}) (interface{}, error) {
				return &resBody, nil
			})
			So(err, ShouldEqual, retErr)
			So(resp, ShouldBeNil)
		})

		Convey("Stream", func() {
			err := u.Stream()(server, stream, streamInfo, func(srv interface{}, ss grpc.ServerStream) error {
				return status.Errorf(codes.Unknown, "another")
			})
			So(err, ShouldEqual, retErr)
		})
	})
}
