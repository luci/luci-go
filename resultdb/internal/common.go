// Copyright 2019 The LUCI Authors.
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

package internal

import (
	"net/http"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"

	"go.chromium.org/luci/grpc/grpcutil"
)

var httpClientCtxKey = "context key for a *http.Client"

// WithHTTPClient returns a context with the client embedded.
func WithHTTPClient(ctx context.Context, client *http.Client) context.Context {
	return context.WithValue(ctx, &httpClientCtxKey, client)
}

// HTTPClient retrieves the current http.client from the context.
func HTTPClient(ctx context.Context) *http.Client {
	client, ok := ctx.Value(&httpClientCtxKey).(*http.Client)
	if !ok {
		panic("no HTTP client in context")
	}
	return client
}

// UnwrapGrpcCodePostlude extracts an error code from a grpcutil.Tag, logs a
// stack trace and returns a gRPC-native error.
// It can be used as Postlude in a decorated gRPC service implementation.
func UnwrapGrpcCodePostlude(ctx context.Context, methodName string, rsp proto.Message, err error) error {
	return grpcutil.GRPCifyAndLogErr(ctx, err)
}
