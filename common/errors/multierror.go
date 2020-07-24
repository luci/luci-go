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
	"fmt"
)

// MultiError is a simple `error` implementation which represents multiple
// `error` objects in one.
type MultiError []error

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

func (m MultiError) stackContext() stackContext {
	n, _ := m.Summary()

	return stackContext{
		internalReason: fmt.Sprintf(
			"MultiError %d/%d: following first non-nil error.", n, len(m)),
	}
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
