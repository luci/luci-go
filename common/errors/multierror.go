// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package errors

import (
	"fmt"
	"reflect"
)

var (
	multiErrorType = reflect.TypeOf(MultiError(nil))
)

// MultiError is a simple `error` implementation which represents multiple
// `error` objects in one.
type MultiError []error

func (m MultiError) Error() string {
	s, n := "", 0
	for _, e := range m {
		if e != nil {
			if n == 0 {
				s = e.Error()
			}
			n++
		}
	}
	switch n {
	case 0:
		return "(0 errors)"
	case 1:
		return s
	case 2:
		return s + " (and 1 other error)"
	}
	return fmt.Sprintf("%s (and %d other errors)", s, n-1)
}

// NewMultiError create new multi error from given errors.
//
// Can be used to workaround 'go vet' confusion "composite literal uses unkeyed
// fields" or if you do not want to remember that MultiError is in fact []error.
func NewMultiError(errors ...error) MultiError {
	return errors
}

// SingleError provides a simple way to uwrap a MultiError if you know that it
// could only ever contain one element.
//
// If err is a MultiError, return its first element. Otherwise, return err.
func SingleError(err error) error {
	if me, ok := err.(MultiError); ok {
		if len(me) == 0 {
			return nil
		}
		return me[0]
	}
	return err
}

// MultiErrorFromErrors takes an error-channel, blocks on it, and returns
// a MultiError for any errors pushed to it over the channel, or nil if
// all the errors were nil.
func MultiErrorFromErrors(ch <-chan error) error {
	if ch == nil {
		return nil
	}
	ret := MultiError(nil)
	for e := range ch {
		if e == nil {
			continue
		}
		ret = append(ret, e)
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

// Fix will convert a MultiError compatible type (e.g. []error) to this
// version of MultiError.
func Fix(err error) error {
	if err != nil {
		// we know that err already conforms to the error interface (or the caller's
		// method wouldn't compile), so check to see if the error's underlying type
		// looks like one of the special error types we implement.
		v := reflect.ValueOf(err)
		if v.Type().ConvertibleTo(multiErrorType) {
			err = v.Convert(multiErrorType).Interface().(error)
		}
	}
	return err
}
