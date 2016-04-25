// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package errors

// Transient is an Error implementation. It wraps an existing Error, marking
// it as transient. This can be tested with IsTransient.
type Transient interface {
	error

	// IsTransient returns true if this error type is transient.
	IsTransient() bool
}

type transientWrapper struct {
	error
}

var _ interface {
	Transient
	Wrapped
} = transientWrapper{}

func (t transientWrapper) IsTransient() bool {
	return true
}

func (t transientWrapper) InnerError() error {
	return t.error
}

// IsTransient tests if a given error or, if it is a container, any of its
// contained errors is Transient.
func IsTransient(err error) bool {
	return Any(err, func(err error) bool {
		if t, ok := err.(Transient); ok {
			return t.IsTransient()
		}
		return false
	})
}

// WrapTransient wraps an existing error with in a Transient error.
//
// If the supplied error is already Transient, it will be returned. If the
// supplied error is nil, nil wil be returned.
func WrapTransient(err error) error {
	if err == nil || IsTransient(err) {
		return err
	}
	return transientWrapper{err}
}
