// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package prpc

import "fmt"

// httpError is an error with HTTP status.
// Many internal functions return *httpError instead of error
// to guarantee expected HTTP status via type-system.
// Supported by ErrorStatus and ErrorDesc.
type httpError struct {
	err    error
	status int
}

func (e *httpError) Error() string {
	return fmt.Sprintf("HTTP %d: %s", e.status, e.err)
}

// withStatus wraps an error with an HTTP status.
// If err is nil, returns nil.
func withStatus(err error, status int) *httpError {
	if _, ok := err.(*httpError); ok {
		panicf("httpError in httpError")
	}
	if err == nil {
		return nil
	}
	return &httpError{err, status}
}

// errorf creates a new error with an HTTP status.
func errorf(status int, format string, a ...interface{}) *httpError {
	return withStatus(fmt.Errorf(format, a...), status)
}

// Errorf creates a new error with an HTTP status.
//
// See also grpc.Errorf that accepts a gRPC code.
func Errorf(status int, format string, a ...interface{}) error {
	return errorf(status, format, a...)
}

func panicf(format string, a ...interface{}) {
	panic(fmt.Errorf(format, a...))
}
