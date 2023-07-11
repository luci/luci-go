// Copyright 2023 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package grpcutil

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// MalformedErrorFixer is a UnifiedServerInterceptor that converts non-nil
// errors with OK status into INTERNAL errors.
//
// It is technically possible for a non-nil error to have OK status code if it
// implements GRPCStatus() that returns nil or OK. This confuses the gRPC client
// (which expect to see a response body), resulting in mysterious io.EOF error.
// There are no known cases when this behavior is desired. This interceptor
// convert such malformed errors into INTERNAL ones. It is present in the
// server's default interceptor chain.
var MalformedErrorFixer = UnifiedServerInterceptor(func(ctx context.Context, fullMethod string, handler func(ctx context.Context) error) error {
	err := handler(ctx)
	if err != nil && status.Code(err) == codes.OK {
		err = status.Errorf(codes.Internal, "BUG: the handler returned a non-nil error with OK status: %s", err)
	}
	return err
})
