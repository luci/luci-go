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

package workunits

import "fmt"

const (
	// ModuleInheritanceStatusNoModuleSet indicates the work unit is not
	// inheriting a module and has no module set.
	ModuleInheritanceStatusNoModuleSet ModuleInheritanceStatus = 1
	// ModuleInheritanceStatusRoot indicates the work unit has set a module
	// and did not inherit this module from its parent. The work unit is the
	// source of inheritance for its children.
	ModuleInheritanceStatusRoot ModuleInheritanceStatus = 2
	// ModuleInheritanceStatusInherited indicates the work unit is inheriting
	// a module from its parent.
	ModuleInheritanceStatusInherited ModuleInheritanceStatus = 3
)

// Represents the module inheritance status of a work unit.
type ModuleInheritanceStatus int64

// String returns the string representation of the ModuleInheritanceStatus.
func (s ModuleInheritanceStatus) String() string {
	switch s {
	case ModuleInheritanceStatusNoModuleSet:
		return "NO_MODULE_SET"
	case ModuleInheritanceStatusRoot:
		return "ROOT"
	case ModuleInheritanceStatusInherited:
		return "INHERITED"
	default:
		return fmt.Sprintf("UNKNOWN (%d)", int64(s))
	}
}
