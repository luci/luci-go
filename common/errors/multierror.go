// Copyright 2015 The LUCI Authors.
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

package errors

import (
	"errors"
	"fmt"
)

// MultiError is a simple `error` implementation which represents multiple
// `error` objects in one.
type MultiError []error

// Unwrap turns MultiError to a slices of errors.
//
// This will make MultiError works with errors.Is or errors.As from stdlib.
func (m MultiError) Unwrap() []error {
	return m
}

// Is implements errors.Is.
//
// This implementation does not interfere with errors.Is, but DOES allow
// matching, especially in tests, when matching a MultiError pattern against
// another MultiError whose size matches exactly and whose contents recursively
// match with `errors.Is`.
//
// This is necessary because the stdlib `errors.Is` will call `Unwrap() []error`
// on the source `err`, but NOT the target `err`, meaning that without this
// method, MultiError can NEVER be the right hand side of a successful
// `errors.Is` call.
//
// If this returns false, `errors.Is` will continue its typical algorithm.
func (m MultiError) Is(other error) bool {
	omerr, ok := other.(MultiError)
	if !ok {
		return false
	}
	if len(omerr) != len(m) {
		return false
	}
	for i, mine := range m {
		if !errors.Is(mine, omerr[i]) {
			return false
		}
	}
	return true
}

// MaybeAdd will add `err` to `m` if `err` is not nil.
func (m *MultiError) MaybeAdd(err error) {
	if err == nil {
		return
	}
	*m = append(*m, err)
}

func (m MultiError) Error() string {
	n, e := m.Summary()
	switch n {
	case 0:
		return "(0 errors)"
	case 1:
		return e.Error()
	case 2:
		return e.Error() + " (and 1 other error)"
	}
	return fmt.Sprintf("%s (and %d other errors)", e, n-1)
}

// AsError returns an `error` interface for this MultiError only if it has >0
// length.
func (m MultiError) AsError() error {
	if len(m) == 0 {
		return nil
	}
	return m
}

// Summary gets the total count of non-nil errors and returns the first one.
func (m MultiError) Summary() (n int, first error) {
	for _, e := range m {
		if e != nil {
			if n == 0 {
				first = e
			}
			n++
		}
	}
	return
}

// First returns the first non-nil error.
func (m MultiError) First() error {
	for _, e := range m {
		if e != nil {
			return e
		}
	}
	return nil
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

// Flatten collapses a multi-dimensional MultiError space into a flat
// MultiError, removing "nil" errors.
//
// If err is not an errors.MultiError, will return err directly.
//
// As a special case, if merr contains no non-nil errors, nil will be returned.
func Flatten(err error) error {
	var ret MultiError
	flattenRec(&ret, err)
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func flattenRec(ret *MultiError, err error) {
	switch et := err.(type) {
	case nil:
	case MultiError:
		for _, e := range et {
			flattenRec(ret, e)
		}
	default:
		*ret = append(*ret, et)
	}
}

// extract removes unnecessary singletons and empty multierrors from a MultiError.
//
// It is not recursive.
func extract(e error) error {
	switch et := e.(type) {
	case nil:
		return nil
	case MultiError:
		switch len(et) {
		case 0:
			return nil
		case 1:
			return et[0]
		}
	}
	return e
}

// Append takes a list of errors, whether they are nil, MultiErrors, or regular errors, and combines them into a single error.
//
// The resulting error is not a multierror unless it needs to be.
//
// Sample usage shown below:
//
//	err := DoSomething()
//	err = errors.Append(err, DoSomethingElse())
//	err = errors.Append(err, DoAThirdThing())
//
//	if err != nil {
//	  // log an error or something, I don't know
//	}
//	// proceed as normal
func Append(errs ...error) error {
	return extract(Flatten(MultiError(errs)))
}
