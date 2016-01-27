// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package prpc

import (
	"net/http"

	"google.golang.org/grpc/codes"
)

// StatusCode maps HTTP statuses to gRPC codes.
// Falls back to codes.Unknown.
//
// The behavior of this function may change when
// https://github.com/grpc/grpc-common/issues/210
// is closed.
func StatusCode(status int) codes.Code {
	switch {

	case status >= 200 && status < 300:
		return codes.OK

	case status == http.StatusUnauthorized:
		return codes.Unauthenticated
	case status == http.StatusForbidden:
		return codes.PermissionDenied
	case status == http.StatusNotFound:
		return codes.NotFound
	case status == http.StatusGone:
		return codes.NotFound
	case status == http.StatusPreconditionFailed:
		return codes.FailedPrecondition
	case status >= 400 && status < 500:
		return codes.InvalidArgument

	case status == http.StatusNotImplemented:
		return codes.Unimplemented
	case status == http.StatusServiceUnavailable:
		return codes.Unavailable
	case status >= 500 && status < 600:
		return codes.Internal

	default:
		return codes.Unknown
	}
}
