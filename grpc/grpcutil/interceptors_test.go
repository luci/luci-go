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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestChainUnaryServerInterceptors(t *testing.T) {
	t.Parallel()

	ftt.Run("With interceptors", t, func(t *ftt.Test) {
		testCtxKey := "testing"
		testInfo := &grpc.UnaryServerInfo{} // constant address for assertions
		testResponse := new(int)            // constant address for assertions
		testError := errors.New("boom")     // constant address for assertions

		calls := []string{}
		record := func(fn string) func() {
			calls = append(calls, "-> "+fn)
			return func() { calls = append(calls, "<- "+fn) }
		}

		callChain := func(intr grpc.UnaryServerInterceptor, h grpc.UnaryHandler) (any, error) {
			return intr(context.Background(), "request", testInfo, h)
		}

		// A "library" of interceptors used below.

		doNothing := func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
			defer record("doNothing")()
			return handler(ctx, req)
		}

		populateContext := func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
			defer record("populateContext")()
			return handler(context.WithValue(ctx, &testCtxKey, "value"), req)
		}

		checkContext := func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
			defer record("checkContext")()
			assert.Loosely(t, ctx.Value(&testCtxKey), should.Equal("value"))
			return handler(ctx, req)
		}

		modifyReq := func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
			defer record("modifyReq")()
			return handler(ctx, "modified request")
		}

		checkReq := func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
			defer record("checkReq")()
			assert.Loosely(t, req.(string), should.Equal("modified request"))
			return handler(ctx, req)
		}

		checkErr := func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
			defer record("checkErr")()
			resp, err := handler(ctx, req)
			assert.Loosely(t, err, should.Equal(testError))
			return resp, err
		}

		abortChain := func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
			defer record("abortChain")()
			return nil, testError
		}

		successHandler := func(ctx context.Context, req any) (any, error) {
			defer record("successHandler")()
			return testResponse, nil
		}

		errorHandler := func(ctx context.Context, req any) (any, error) {
			defer record("errorHandler")()
			return nil, testError
		}

		t.Run("Noop chain", func(t *ftt.Test) {
			resp, err := callChain(ChainUnaryServerInterceptors(nil, nil), successHandler)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp, should.Equal(testResponse))
			assert.Loosely(t, calls, should.Match([]string{
				"-> successHandler",
				"<- successHandler",
			}))
		})

		t.Run("One link chain", func(t *ftt.Test) {
			resp, err := callChain(ChainUnaryServerInterceptors(doNothing), successHandler)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp, should.Equal(testResponse))
			assert.Loosely(t, calls, should.Match([]string{
				"-> doNothing",
				"-> successHandler",
				"<- successHandler",
				"<- doNothing",
			}))
		})

		t.Run("Nils are OK", func(t *ftt.Test) {
			resp, err := callChain(ChainUnaryServerInterceptors(nil, doNothing, nil, nil), successHandler)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp, should.Equal(testResponse))
			assert.Loosely(t, calls, should.Match([]string{
				"-> doNothing",
				"-> successHandler",
				"<- successHandler",
				"<- doNothing",
			}))
		})

		t.Run("Changes propagate", func(t *ftt.Test) {
			chain := ChainUnaryServerInterceptors(
				populateContext,
				modifyReq,
				doNothing,
				checkContext,
				checkReq,
			)
			resp, err := callChain(chain, successHandler)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp, should.Equal(testResponse))
			assert.Loosely(t, calls, should.Match([]string{
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
			}))
		})

		t.Run("Request error propagates", func(t *ftt.Test) {
			chain := ChainUnaryServerInterceptors(
				doNothing,
				checkErr,
			)
			_, err := callChain(chain, errorHandler)
			assert.Loosely(t, err, should.Equal(testError))
			assert.Loosely(t, calls, should.Match([]string{
				"-> doNothing",
				"-> checkErr",
				"-> errorHandler",
				"<- errorHandler",
				"<- checkErr",
				"<- doNothing",
			}))
		})

		t.Run("Interceptor can abort the chain", func(t *ftt.Test) {
			chain := ChainUnaryServerInterceptors(
				doNothing,
				abortChain,
				doNothing,
				doNothing,
				doNothing,
				doNothing,
			)
			_, err := callChain(chain, successHandler)
			assert.Loosely(t, err, should.Equal(testError))
			assert.Loosely(t, calls, should.Match([]string{
				"-> doNothing",
				"-> abortChain",
				"<- abortChain",
				"<- doNothing",
			}))
		})
	})
}

func TestChainStreamServerInterceptors(t *testing.T) {
	t.Parallel()

	// Note: this is 80% copy-pasta of TestChainUnaryServerInterceptors just using
	// different types to match StreamServerInterceptor API.

	ftt.Run("With interceptors", t, func(t *ftt.Test) {
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

		doNothing := func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
			defer record("doNothing")()
			return handler(srv, ss)
		}

		populateContext := func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
			defer record("populateContext")()
			return handler(srv, ModifyServerStreamContext(ss, func(ctx context.Context) context.Context {
				return context.WithValue(ctx, &testCtxKey, "value")
			}))
		}

		checkContext := func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
			defer record("checkContext")()
			assert.Loosely(t, ss.Context().Value(&testCtxKey), should.Equal("value"))
			return handler(srv, ss)
		}

		modifySrv := func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
			defer record("modifySrv")()
			return handler("modified srv", ss)
		}

		checkSrv := func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
			defer record("checkSrv")()
			assert.Loosely(t, srv.(string), should.Equal("modified srv"))
			return handler(srv, ss)
		}

		checkErr := func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
			defer record("checkErr")()
			err := handler(srv, ss)
			assert.Loosely(t, err, should.Equal(testError))
			return err
		}

		abortChain := func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
			defer record("abortChain")()
			return testError
		}

		successHandler := func(srv any, ss grpc.ServerStream) error {
			defer record("successHandler")()
			return nil
		}

		errorHandler := func(srv any, ss grpc.ServerStream) error {
			defer record("errorHandler")()
			return testError
		}

		t.Run("Noop chain", func(t *ftt.Test) {
			err := callChain(ChainStreamServerInterceptors(nil, nil), successHandler)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, calls, should.Match([]string{
				"-> successHandler",
				"<- successHandler",
			}))
		})

		t.Run("One link chain", func(t *ftt.Test) {
			err := callChain(ChainStreamServerInterceptors(doNothing), successHandler)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, calls, should.Match([]string{
				"-> doNothing",
				"-> successHandler",
				"<- successHandler",
				"<- doNothing",
			}))
		})

		t.Run("Nils are OK", func(t *ftt.Test) {
			err := callChain(ChainStreamServerInterceptors(nil, doNothing, nil, nil), successHandler)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, calls, should.Match([]string{
				"-> doNothing",
				"-> successHandler",
				"<- successHandler",
				"<- doNothing",
			}))
		})

		t.Run("Changes propagate", func(t *ftt.Test) {
			chain := ChainStreamServerInterceptors(
				populateContext,
				modifySrv,
				doNothing,
				checkContext,
				checkSrv,
			)
			err := callChain(chain, successHandler)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, calls, should.Match([]string{
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
			}))
		})

		t.Run("Request error propagates", func(t *ftt.Test) {
			chain := ChainStreamServerInterceptors(
				doNothing,
				checkErr,
			)
			err := callChain(chain, errorHandler)
			assert.Loosely(t, err, should.Equal(testError))
			assert.Loosely(t, calls, should.Match([]string{
				"-> doNothing",
				"-> checkErr",
				"-> errorHandler",
				"<- errorHandler",
				"<- checkErr",
				"<- doNothing",
			}))
		})

		t.Run("Interceptor can abort the chain", func(t *ftt.Test) {
			chain := ChainStreamServerInterceptors(
				doNothing,
				abortChain,
				doNothing,
				doNothing,
				doNothing,
				doNothing,
			)
			err := callChain(chain, successHandler)
			assert.Loosely(t, err, should.Equal(testError))
			assert.Loosely(t, calls, should.Match([]string{
				"-> doNothing",
				"-> abortChain",
				"<- abortChain",
				"<- doNothing",
			}))
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

	ftt.Run("Passes requests, modifies the context", t, func(t *ftt.Test) {
		var u UnifiedServerInterceptor = func(ctx context.Context, fullMethod string, handler func(ctx context.Context) error) error {
			assert.Loosely(t, ctx, should.Equal(rootCtx))
			assert.Loosely(t, fullMethod, should.Equal("/svc/method"))
			return handler(context.WithValue(ctx, key("key"), "val"))
		}

		t.Run("Unary", func(t *ftt.Test) {
			resp, err := u.Unary()(rootCtx, &reqBody, unaryInfo, func(ctx context.Context, req any) (any, error) {
				assert.Loosely(t, ctx.Value(key("key")).(string), should.Equal("val"))
				assert.Loosely(t, req, should.Equal(&reqBody))
				return &resBody, nil
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp, should.Equal(&resBody))
		})

		t.Run("Stream", func(t *ftt.Test) {
			err := u.Stream()(server, stream, streamInfo, func(srv any, ss grpc.ServerStream) error {
				assert.Loosely(t, srv, should.Equal(server))
				assert.Loosely(t, ss.Context().Value(key("key")).(string), should.Equal("val"))
				return nil
			})
			assert.Loosely(t, err, should.BeNil)
		})
	})

	ftt.Run("Sees errors", t, func(t *ftt.Test) {
		retErr := status.Errorf(codes.Unknown, "boo")
		var seenErr error

		var u UnifiedServerInterceptor = func(ctx context.Context, fullMethod string, handler func(ctx context.Context) error) error {
			seenErr = handler(ctx)
			return seenErr
		}

		t.Run("Unary", func(t *ftt.Test) {
			resp, err := u.Unary()(rootCtx, &reqBody, unaryInfo, func(ctx context.Context, req any) (any, error) {
				return &resBody, retErr
			})
			assert.Loosely(t, err, should.Equal(retErr))
			assert.Loosely(t, seenErr, should.Equal(retErr))
			assert.Loosely(t, resp, should.BeNil)
		})

		t.Run("Stream", func(t *ftt.Test) {
			err := u.Stream()(server, stream, streamInfo, func(srv any, ss grpc.ServerStream) error {
				return retErr
			})
			assert.Loosely(t, err, should.Equal(retErr))
			assert.Loosely(t, seenErr, should.Equal(retErr))
		})
	})

	ftt.Run("Can block requests", t, func(t *ftt.Test) {
		retErr := status.Errorf(codes.Unknown, "boo")

		var u UnifiedServerInterceptor = func(ctx context.Context, fullMethod string, handler func(ctx context.Context) error) error {
			return retErr
		}

		t.Run("Unary", func(t *ftt.Test) {
			resp, err := u.Unary()(rootCtx, &reqBody, unaryInfo, func(ctx context.Context, req any) (any, error) {
				panic("must not be called")
			})
			assert.Loosely(t, err, should.Equal(retErr))
			assert.Loosely(t, resp, should.BeNil)
		})

		t.Run("Stream", func(t *ftt.Test) {
			err := u.Stream()(server, stream, streamInfo, func(srv any, ss grpc.ServerStream) error {
				panic("must not be called")
			})
			assert.Loosely(t, err, should.Equal(retErr))
		})
	})

	ftt.Run("Can override error", t, func(t *ftt.Test) {
		retErr := status.Errorf(codes.Unknown, "boo")

		var u UnifiedServerInterceptor = func(ctx context.Context, fullMethod string, handler func(ctx context.Context) error) error {
			_ = handler(ctx)
			return retErr
		}

		t.Run("Unary", func(t *ftt.Test) {
			resp, err := u.Unary()(rootCtx, &reqBody, unaryInfo, func(ctx context.Context, req any) (any, error) {
				return &resBody, nil
			})
			assert.Loosely(t, err, should.Equal(retErr))
			assert.Loosely(t, resp, should.BeNil)
		})

		t.Run("Stream", func(t *ftt.Test) {
			err := u.Stream()(server, stream, streamInfo, func(srv any, ss grpc.ServerStream) error {
				return status.Errorf(codes.Unknown, "another")
			})
			assert.Loosely(t, err, should.Equal(retErr))
		})
	})
}
