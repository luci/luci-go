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

package fs

// IsNotEmpty is true if err represents ENOTEMPTY or equivalent.
func IsNotEmpty(err error) bool {
	return isNotEmpty(err) // see either fs_posix.go or fs_windows.go
}

// IsNotDir if true if err represents ENOTDIR or equivalent.
func IsNotDir(err error) bool {
	return isNotDir(err) // see either fs_posix.go or fs_windows.go
}
