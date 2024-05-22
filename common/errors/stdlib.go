// Copyright 2022 The LUCI Authors.
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
)

// ErrUnsupported re-exports [errors.ErrUnsupported] from the standard library.
var ErrUnsupported = errors.ErrUnsupported

// Is re-exports [errors.Is] from the standard library.
func Is(e error, target error) bool {
	return errors.Is(e, target)
}

// As re-exports [errors.As] from the standard library.
func As(e error, target any) bool {
	return errors.As(e, target)
}

// Join re-exports [errors.Join] from the standard library.
func Join(errs ...error) error {
	return errors.Join(errs...)
}
