// Copyright 2017 The LUCI Authors.
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

package milo

// IsEmulated returns true iff the given buildNumber should be emulated.
func (m *EmulationOptions) IsEmulated(buildNumber int) bool {
	if m == nil || m.StartFrom < 0 {
		return false
	}
	return buildNumber >= int(m.StartFrom)
}
