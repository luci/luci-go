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

	s := statusFromError(err)
	if s.Code() == codes.Internal || s.Code() == codes.Unknown {
		errors.Log(ctx, err)
	}
	return s.Err()
}

// statusFromError returns a status to return to the client based on the error.
func statusFromError(err error) *status.Status {
	if s, ok := Get(err); ok {
		return s
	}

	if err := errors.Unwrap(err); err == context.DeadlineExceeded || err == context.Canceled {
		return status.FromContextError(err)
	}

	return status.New(codes.Internal, "internal server error")
}
