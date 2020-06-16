// Copyright 2016 The LUCI Authors.
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

// Wrapped indicates an error that wraps another error.
type Wrapped interface {
	// Unwrap returns the wrapped error.
	Unwrap() error
}

// Unwrap unwraps a wrapped error recursively, returning its inner error.
//
// If the supplied error is not nil, Unwrap will never return nil. If a
// wrapped error reports that its Unwrap is nil, that error will be
// returned.
func Unwrap(err error) error {
	for {
		wrap, ok := err.(Wrapped)
		if !ok {
			return err
		}

		inner := wrap.Unwrap()
		if inner == nil {
			break
		}
		err = inner
	}
	return err
}
