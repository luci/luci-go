// Copyright 2026 The LUCI Authors.
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

package perm

// TODO: Register remaining CIPD realm permissions and attach group/role
// mappings.
var (
	DiagnoseACL = definePerm("cipd.packages.diagnoseACL")

	// AllPermissions is the set of all Grantables defined in this package.
	AllPermissions = map[Grantable]struct{}{}
)

// AllPermissionsSet returns [AllPermissions] as a [Checkable] [Set].
//
// Note that for bindable permissions, this contains their unconditional
// variants.
func AllPermissionsSet() Set {
	ret := make(Set, len(AllPermissions))
	for perm := range AllPermissions {
		ret.Add(Permission{core: perm.(*corePermission)})
	}
	return ret
}
