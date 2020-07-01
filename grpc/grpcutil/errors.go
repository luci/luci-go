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

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
)

var (
	// Errf falls through to status.Errorf, with the notable exception that it isn't
	// named "Errorf" and, consequently, won't trigger "go vet" misuse errors.
	Errf = status.Errorf

	// OK is an empty grpc.OK status error.
	OK = Errf(codes.OK, "")

	// Canceled is an empty grpc.Canceled error.
	Canceled = Errf(codes.Canceled, "")

	// Unknown is an empty grpc.Unknown error.
	Unknown = Errf(codes.Unknown, "")

	// InvalidArgument is an empty grpc.InvalidArgument error.
	InvalidArgument = Errf(codes.InvalidArgument, "")

	// DeadlineExceeded is an empty grpc.DeadlineExceeded error.
	DeadlineExceeded = Errf(codes.DeadlineExceeded, "")

	// NotFound is an empty grpc.NotFound error.
	NotFound = Errf(codes.NotFound, "")

	// AlreadyExists is an empty grpc.AlreadyExists error.
	AlreadyExists = Errf(codes.AlreadyExists, "")

	// PermissionDenied is an empty grpc.PermissionDenied error.
	PermissionDenied = Errf(codes.PermissionDenied, "")

	// Unauthenticated is an empty grpc.Unauthenticated error.
	Unauthenticated = Errf(codes.Unauthenticated, "")

	// ResourceExhausted is an empty grpc.ResourceExhausted error.
	ResourceExhausted = Errf(codes.ResourceExhausted, "")

	// FailedPrecondition is an empty grpc.FailedPrecondition error.
	FailedPrecondition = Errf(codes.FailedPrecondition, "")

	// Aborted is an empty grpc.Aborted error.
	Aborted = Errf(codes.Aborted, "")

	// OutOfRange is an empty grpc.OutOfRange error.
	OutOfRange = Errf(codes.OutOfRange, "")

	// Unimplemented is an empty grpc.Unimplemented error.
	Unimplemented = Errf(codes.Unimplemented, "")

	// Internal is an empty grpc.Internal error.
	Internal = Errf(codes.Internal, "")

	// Unavailable is an empty grpc.Unavailable error.
	Unavailable = Errf(codes.Unavailable, "")

	// DataLoss is an empty grpc.DataLoss error.
	DataLoss = Errf(codes.DataLoss, "")
)

// WrapIfTransient wraps the supplied gRPC error with a transient wrapper if
// it has a transient gRPC code, as determined by IsTransientCode.
//
// If the supplied error is nil, nil will be returned.
//
// Note that non-gRPC errors will have code grpc.Unknown, which is considered
// transient, and be wrapped. This function should only be used on gRPC errors.
func WrapIfTransient(err error) error {
	if err == nil {
		return nil
	}

	if IsTransientCode(Code(err)) {
		err = transient.Tag.Apply(err)
	}
	return err
}

type grpcCodeTag struct{ Key errors.TagKey }

func (g grpcCodeTag) With(code codes.Code) errors.TagValue {
	return errors.TagValue{Key: g.Key, Value: code}
}
func (g grpcCodeTag) In(err error) (v codes.Code, ok bool) {
	d, ok := errors.TagValueIn(g.Key, err)
	if ok {
		v = d.(codes.Code)
	}
	return
}

// Tag may be used to associate a gRPC status code with this error.
//
// The tag value MUST be a "google.golang.org/grpc/codes".Code.
var Tag = grpcCodeTag{errors.NewTagKey("gRPC Code")}

// Shortcuts for assigning tags with codes known at compile time.
//
// Instead errors.Annotate(...).Tag(grpcutil.Tag.With(codes.InvalidArgument)) do
// errors.Annotate(...).Tag(grpcutil.InvalidArgumentTag)).
var (
	CanceledTag           = Tag.With(codes.Canceled)
	UnknownTag            = Tag.With(codes.Unknown)
	InvalidArgumentTag    = Tag.With(codes.InvalidArgument)
	DeadlineExceededTag   = Tag.With(codes.DeadlineExceeded)
	NotFoundTag           = Tag.With(codes.NotFound)
	AlreadyExistsTag      = Tag.With(codes.AlreadyExists)
	PermissionDeniedTag   = Tag.With(codes.PermissionDenied)
	UnauthenticatedTag    = Tag.With(codes.Unauthenticated)
	ResourceExhaustedTag  = Tag.With(codes.ResourceExhausted)
	FailedPreconditionTag = Tag.With(codes.FailedPrecondition)
	AbortedTag            = Tag.With(codes.Aborted)
	OutOfRangeTag         = Tag.With(codes.OutOfRange)
	UnimplementedTag      = Tag.With(codes.Unimplemented)
	InternalTag           = Tag.With(codes.Internal)
	UnavailableTag        = Tag.With(codes.Unavailable)
	DataLossTag           = Tag.With(codes.DataLoss)
)

// codeToStatus maps gRPC codes to HTTP statuses.
// Based on https://cloud.google.com/apis/design/errors
var codeToStatus = map[codes.Code]int{
	codes.OK:                 http.StatusOK,
	codes.Canceled:           499,
	codes.InvalidArgument:    http.StatusBadRequest,
	codes.DataLoss:           http.StatusGone,
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
// Falls back to http.StatusInternalServerError.
func CodeStatus(code codes.Code) int {
	if status, ok := codeToStatus[code]; ok {
		return status
	}
	return http.StatusInternalServerError
}

// Code returns the gRPC code for a given error.
//
// In addition to the functionality of grpc.Code, this will unwrap any wrapped
// errors before asking for its code.
//
// If the error is a MultiError containing more than one type of error code,
// this will return codes.Unknown.
func Code(err error) codes.Code {
	if code, ok := Tag.In(err); ok {
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
	// TODO(nodir): use status.FromError
	return grpc.Code(errors.Unwrap(err))
}

// ToGRPCErr is a shorthand for Errf(Code(err), "%s", err)
func ToGRPCErr(err error) error {
	return Errf(Code(err), "%s", err)
}

// IsTransientCode returns true if a given gRPC code is associated with a
// transient gRPC error type.
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
//   func (...) Method(...) (resp *pb.Response, err error) {
//     defer func() { err = grpcutil.GRPCifyAndLogErr(c, err) }()
//     ...
//   }
func GRPCifyAndLogErr(c context.Context, err error) error {
	if err == nil {
		return nil
	}
	if _, yep := status.FromError(err); yep {
		return err
	}
	grpcErr := ToGRPCErr(err)
	code := grpc.Code(grpcErr)
	if code == codes.Internal || code == codes.Unknown {
		errors.Log(c, err)
	}
	return grpcErr
}
