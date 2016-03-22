// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package grpcutil

import (
	"github.com/luci/luci-go/common/logging"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"golang.org/x/net/context"
)

var (
	// Errf falls through to grpc.Errorf, with the notable exception that it isn't
	// named "Errorf" and, consequently, won't trigger "go vet" misuse errors.
	Errf = grpc.Errorf

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

// IsTransient returns true if a given gRPC error is transient.
//
// Note that this will return true for non-gRPC error types, since they resolve
// to codes.Unavailable, which is considered transient by IsTransientCode.
func IsTransient(err error) bool {
	return err != nil && IsTransientCode(grpc.Code(err))
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

// LogErr logs the non-nil error and transforms it into a grpc error with the
// given code. If the err is nil, this returns nil without logging anything.
//
// If the code is InvalidArgument error message will be passed through.
// Otherwise the actual content of `err` will be omitted.
//
// InvalidArgument, Unauthenticated and DeadlineExceeded are logged as 'Info'
// level. All other error codes are logged as 'Error' level.
func LogErr(c context.Context, code codes.Code, err error, msg string) error {
	if err == nil {
		return nil
	}
	log := logging.Fields.Errorf
	switch code {
	case codes.InvalidArgument, codes.Unauthenticated, codes.DeadlineExceeded:
		log = logging.Fields.Infof
	}
	log(logging.Fields{
		logging.ErrorKey: err,
		"grpc_code":      code,
	}, c, "%s", msg)
	if code == codes.InvalidArgument {
		return Errf(code, "%s", err)
	}
	return Errf(code, "")
}
