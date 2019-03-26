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

package common

import "sort"

// MergeStrings merges multiple string slices together into a single slice,
// removing duplicates.
func MergeStrings(sss ...[]string) []string {
	result := []string{}
	seen := map[string]bool{}
	for _, ss := range sss {
		for _, s := range ss {
			if seen[s] {
				continue
			}
			seen[s] = true
			result = append(result, s)
		}
	}
	sort.Strings(result)
	return result
}
