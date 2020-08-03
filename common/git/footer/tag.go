// Copyright 2020 The LUCI Authors.
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

package footer

import (
	"regexp"
	"strings"

	"go.chromium.org/luci/common/data/strpair"
)

var metadataPattern = regexp.MustCompile(`^([A-Z_]+)=(.*)$`)

// ParseLegacyMetadata extracts legacy style `<KEY>=<value>` metadata from
// the given message. Unlike git footer, this works across *all* lines
// instead of the last paragraph only.
// Returns a multimap as a key may map to multiple values. The metadata defined
// in a latter line takes precedence and shows up at the front of the value
// slice.
func ParseLegacyMetadata(message string) strpair.Map {
	lines := strings.Split(message, "\n")
	ret := strpair.Map{}
	for i := len(lines) - 1; i >= 0; i-- {
		res := metadataPattern.FindStringSubmatch(lines[i])
		if len(res) == 3 {
			ret.Add(res[1], strings.TrimSpace(res[2]))
		}
	}
	return ret
}
