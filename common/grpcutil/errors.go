// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package grpcutil

import (
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
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
