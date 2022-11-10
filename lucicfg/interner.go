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

package lucicfg

// stringInterner can be used to save memory by interning strings.
type stringInterner map[string]string

// internString returns an interned copy of `s`.
func (si stringInterner) internString(s string) string {
	if interned, ok := si[s]; ok {
		return interned
	}
	si[s] = s
	return s
}
