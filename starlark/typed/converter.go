// Copyright 2019 The LUCI Authors.
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

package typed

import (
	"go.starlark.net/starlark"
)

// Converter can convert values to a some Starlark type or reject them as
// incompatible.
//
// Must be idempotent, i.e. the following must not panic for all 'x':
//
//	y, err := Convert(x)
//	if err == nil {
//	  z, err := Convert(y)
//	  if err != nil {
//	    panic("converted to an incompatible type")
//	  }
//	  if z != y {
//	    panic("doesn't pass through already converted item")
//	  }
//	}
//
// Must be stateless. Must not mutate the values being converted.
type Converter interface {
	// Convert takes a value and either returns it as is (if it already has the
	// necessary type) or allocates a new value of necessary type and populates
	// it based on data in 'x'.
	//
	// Returns an error if 'x' can't be converted.
	Convert(x starlark.Value) (starlark.Value, error)

	// Type returns a name of the type the converter converts to.
	//
	// Used only to construct composite type names such as "list<T>".
	Type() string
}
