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

package util

import (
	"fmt"
	"regexp"
)

// MatchedNamedGroup matches a string against a regex with named group
// and returns a mapping between the named group and the matches
func MatchedNamedGroup(r *regexp.Regexp, s string) (map[string]string, error) {
	names := r.SubexpNames()
	matches := r.FindStringSubmatch(s)
	result := make(map[string]string)
	if matches != nil {
		for i, name := range names {
			if name != "" {
				result[name] = matches[i]
			}
		}
		return result, nil
	}
	return nil, fmt.Errorf("Could not find matches")
}
