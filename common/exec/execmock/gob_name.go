// Copyright 2023 The LUCI Authors.
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

package execmock

import (
	"fmt"
	"reflect"
)

// gobName computes the serializable name for the Runner type.
//
// This is copied from gob.Register, minus the bug documented there.
func gobName(value any) string {
	// Default to printed representation for unnamed types
	rt := reflect.TypeOf(value)
	if rt.Kind() == reflect.Interface || rt.Kind() == reflect.Func {
		panic(fmt.Sprintf("invalid Runner In or Out kind: %s", rt.Kind()))
	}
	name := rt.String()

	// But for named types (or pointers to them), qualify with import path (but see inner comment).
	// Dereference one pointer looking for a named type.
	star := ""
	if rt.Name() == "" {
		if pt := rt; pt.Kind() == reflect.Ptr {
			star = "*"
			// This is intentionally different from gob.Register.
			// See the comment there for why.
			rt = pt.Elem()
		}
	}
	if rt.Name() != "" {
		if rt.PkgPath() == "" {
			name = fmt.Sprintf("%s%s", star, rt.Name())
		} else {
			name = fmt.Sprintf("%s%s.%s", star, rt.PkgPath(), rt.Name())
		}
	}
	return name
}
