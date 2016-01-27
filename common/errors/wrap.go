// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package errors

// Wrapped indicates an error that wraps another error.
type Wrapped interface {
	// InnerError returns the wrapped error.
	InnerError() error
}

// Unwrap returns the inner error of err.
// Returns nil if err does not wrap anything or err is nil.
func Unwrap(err error) error {
	if wrap, ok := err.(Wrapped); ok {
		return wrap.InnerError()
	}
	return nil
}

// UnwrapAll unwraps a wrapped error recursively.
// Returns nil iff err is nil.
func UnwrapAll(err error) error {
	for {
		inner := Unwrap(err)
		if inner == nil {
			break
		}
		err = inner
	}
	return err
}
