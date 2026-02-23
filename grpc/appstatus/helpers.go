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

package appstatus

import (
	"context"

	"github.com/golang/protobuf/proto"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
)

// BadRequest annotates err as a bad request.
// The error message is shared with the requester as is.
func BadRequest(err error, details ...*errdetails.BadRequest) error {
	s := status.Newf(codes.InvalidArgument, "bad request: %s", err)

	if len(details) > 0 {
		det := make([]proto.Message, len(details))
		for i, d := range details {
			det[i] = d
		}

		s = MustWithDetails(s, det...)
	}

	return Attach(err, s)
}

// MustWithDetails adds details to a status and asserts it is successful.
func MustWithDetails(s *status.Status, details ...proto.Message) *status.Status {
	s, err := s.WithDetails(details...)
	if err != nil {
		panic(err)
	}
	return s
}

// GRPCifyAndLog returns a GRPC error. If the error doesn't have a GRPC status
// attached by this package, internal error is assumed. Any internal or unknown
// errors are logged.
func GRPCifyAndLog(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}

	s := statusFromError(ctx, err)
	if s.Code() == codes.Internal || s.Code() == codes.Unknown {
		errors.Log(ctx, err)
	}
	return s.Err()
}

// statusFromError returns a status to return to the client based on the error.
func statusFromError(ctx context.Context, err error) *status.Status {
	if s, ok := Get(err); ok {
		return s
	}

	if ctxErr := ctx.Err(); ctxErr == context.DeadlineExceeded || ctxErr == context.Canceled {
		// Fallback: if the context has been cancelled or the deadline was exceeded,
		// RPC handlers often do not return an appstatus-annotated error. Check
		// the error the RPC returned could plausibly be from this context error.
		// If it is, return a corresponding status.

		bubblingUpContextError := errors.Is(err, ctxErr)
		if !bubblingUpContextError {
			// Alternatively, if a nested RPC call returned codes.Cancelled or
			// codes.DeadlineExceeded, and this matches the context error, we are
			// bubbling up the context error.
			code := status.Code(err)
			bubblingUpContextError = ((code == codes.Canceled && ctxErr == context.Canceled) ||
				(code == codes.DeadlineExceeded && ctxErr == context.DeadlineExceeded))
		}

		// The context error was bubbled up.
		if bubblingUpContextError {
			// The original error is not appstatus-annotated so it could contain
			// secrets the client is not allowed to know about; log the original
			// error here for debugging but do not return it. Instead return the
			// context error (or its cause, if available).
			if ctxErr == context.DeadlineExceeded {
				logging.Warningf(ctx, "Returning request deadline exceeded error; original error: %s", err)
				return status.New(codes.DeadlineExceeded, context.Cause(ctx).Error())
			} else {
				// ctxErr == context.Canceled
				logging.Warningf(ctx, "Returning request cancelled error; original error: %s", err)
				return status.New(codes.Canceled, context.Cause(ctx).Error())
			}
		}
	}
	return status.New(codes.Internal, "internal server error")
}
