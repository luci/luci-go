// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package errors

// Transient is an Error implementation. It wraps an existing Error, marking
// it as transient. This can be tested with IsTransient.
type Transient struct {
	Err error
}

// Error implements the error interface.
func (t Transient) Error() string {
	return t.Err.Error()
}

// IsTransient tests if a given error is Transient.
func IsTransient(err error) bool {
	_, ok := err.(Transient)
	return ok
}
