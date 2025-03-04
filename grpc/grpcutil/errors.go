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

package grpcutil

import (
	"context"
	"net/http"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/errors/errtag"
	"go.chromium.org/luci/common/retry/transient"
)

// WrapIfTransient wraps the supplied gRPC error with a transient wrapper if
// its gRPC code is transient as determined by IsTransientCode.
//
// If the supplied error is nil, nil will be returned.
//
// Note that non-gRPC errors will have code codes.Unknown, which is considered
// transient, and be wrapped. This function should only be used on gRPC errors.
//
// Also note that codes.DeadlineExceeded is not considered a transient error,
// since it is often non-retriable (e.g. if the root context has expired, no
// amount of retries will resolve codes.DeadlineExceeded error).
func WrapIfTransient(err error) error {
	if err == nil {
		return nil
	}

	if IsTransientCode(Code(err)) {
		return transient.Tag.Apply(err)
	}

	return err
}

// WrapIfTransientOr wraps the supplied gRPC error with a transient wrapper if
// its gRPC code is transient as determined by IsTransientCode or matches any
// of given `extra` codes.
//
// See WrapIfTransient for other caveats.
func WrapIfTransientOr(err error, extra ...codes.Code) error {
	if err == nil {
		return nil
	}

	code := Code(err)
	if IsTransientCode(code) {
		return transient.Tag.Apply(err)
	}

	for _, c := range extra {
		if code == c {
			return transient.Tag.Apply(err)
		}
	}

	return err
}

// Tag may be used to associate a gRPC status code with this error.
//
// The tag value MUST be a "google.golang.org/grpc/codes".Code.
var Tag = errtag.Make("gRPC Code", codes.Unknown)

// Shortcuts for assigning tags with codes known at compile time.
//
// Instead of Tag.SetValue(err, codes.InvalidArgument), you can do
// codes.InvalidArgumentTag.Apply(err).
var (
	CanceledTag           = Tag.WithDefault(codes.Canceled)
	UnknownTag            = Tag.WithDefault(codes.Unknown)
	InvalidArgumentTag    = Tag.WithDefault(codes.InvalidArgument)
	DeadlineExceededTag   = Tag.WithDefault(codes.DeadlineExceeded)
	NotFoundTag           = Tag.WithDefault(codes.NotFound)
	AlreadyExistsTag      = Tag.WithDefault(codes.AlreadyExists)
	PermissionDeniedTag   = Tag.WithDefault(codes.PermissionDenied)
	UnauthenticatedTag    = Tag.WithDefault(codes.Unauthenticated)
	ResourceExhaustedTag  = Tag.WithDefault(codes.ResourceExhausted)
	FailedPreconditionTag = Tag.WithDefault(codes.FailedPrecondition)
	AbortedTag            = Tag.WithDefault(codes.Aborted)
	OutOfRangeTag         = Tag.WithDefault(codes.OutOfRange)
	UnimplementedTag      = Tag.WithDefault(codes.Unimplemented)
	InternalTag           = Tag.WithDefault(codes.Internal)
	UnavailableTag        = Tag.WithDefault(codes.Unavailable)
	DataLossTag           = Tag.WithDefault(codes.DataLoss)
)

// codeToStatus maps gRPC codes to HTTP statuses.
// Based on https://cloud.google.com/apis/design/errors
var codeToStatus = map[codes.Code]int{
	codes.OK:                 http.StatusOK,
	codes.Canceled:           499,
	codes.InvalidArgument:    http.StatusBadRequest,
	codes.DataLoss:           http.StatusInternalServerError,
	codes.Internal:           http.StatusInternalServerError,
	codes.Unknown:            http.StatusInternalServerError,
	codes.DeadlineExceeded:   http.StatusGatewayTimeout,
	codes.NotFound:           http.StatusNotFound,
	codes.AlreadyExists:      http.StatusConflict,
	codes.PermissionDenied:   http.StatusForbidden,
	codes.Unauthenticated:    http.StatusUnauthorized,
	codes.ResourceExhausted:  http.StatusTooManyRequests,
	codes.FailedPrecondition: http.StatusBadRequest,
	codes.OutOfRange:         http.StatusBadRequest,
	codes.Unimplemented:      http.StatusNotImplemented,
	codes.Unavailable:        http.StatusServiceUnavailable,
	codes.Aborted:            http.StatusConflict,
}

// CodeStatus maps gRPC codes to HTTP status codes.
//
// Falls back to http.StatusInternalServerError if the code is unrecognized.
func CodeStatus(code codes.Code) int {
	if status, ok := codeToStatus[code]; ok {
		return status
	}
	return http.StatusInternalServerError
}

// Code returns the gRPC code for a given error.
//
// In addition to the functionality of status.Code, this will unwrap any wrapped
// errors before asking for its code.
//
// If the error is a MultiError containing more than one type of error code,
// this will return codes.Unknown.
func Code(err error) codes.Code {
	if code, ok := Tag.Value(err); ok {
		return code
	}
	// If it's a multi-error, see if all errors have the same code.
	// Otherwise return codes.Unknown.
	if multi, isMulti := err.(errors.MultiError); isMulti {
		code := codes.OK
		for _, err := range multi {
			nextCode := Code(err)
			if code == codes.OK { // unset
				code = nextCode
				continue
			}
			if nextCode != code {
				return codes.Unknown
			}
		}
		return code
	}
	return status.Code(errors.Unwrap(err))
}

// IsTransientCode returns true if a given gRPC code is codes.Internal,
// codes.Unknown or codes.Unavailable.
func IsTransientCode(code codes.Code) bool {
	switch code {
	case codes.Internal, codes.Unknown, codes.Unavailable:
		return true

	default:
		return false
	}
}

// GRPCifyAndLogErr converts an annotated LUCI error to a gRPC error and logs
// internal details (including stack trace) for errors with Internal or Unknown
// codes.
//
// If err is already gRPC error (or nil), it is silently passed through, even
// if it is Internal. There's nothing interesting to log in this case.
//
// Intended to be used in defer section of gRPC handlers like so:
//
//	func (...) Method(...) (resp *pb.Response, err error) {
//	  defer func() { err = grpcutil.GRPCifyAndLogErr(c, err) }()
//	  ...
//	}
func GRPCifyAndLogErr(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if _, yep := status.FromError(err); yep {
		return err
	}
	code := Code(err)
	if code == codes.Internal || code == codes.Unknown {
		errors.Log(ctx, err)
	}
	return status.Error(code, err.Error())
}
