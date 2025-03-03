// Copyright 2025 The LUCI Authors.
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

// Package errtag provides functionality to add metadata (tag+value pairs) to
// errors without affecting either their queryability (via errors.Is or
// errors.As) or their rendering.
//
// Example:
//
//	type Flavor int
//
//	const (
//		Bland Flavor = iota
//		Sweet
//		Salty
//		Savory
//		Sour
//	)
//
//	// FlavorTag has the default value Bland.
//	var FlavorTag = errtag.Make("somepkg.Flavor", Bland)
//
//	func something() error {
//		var err error
//		...
//		return FlavorTag.ApplyValue(err, Savory)
//	}
//
//	func otherFunc() {
//		err := something()
//		if FlavorTag.ValueOrDefault(err) == Savory {
//			...
//		}
//	}
package errtag
