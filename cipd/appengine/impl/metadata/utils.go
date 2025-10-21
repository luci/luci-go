// Copyright 2018 The LUCI Authors.
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

package metadata

import (
	"go.chromium.org/luci/common/data/stringset"

	repopb "go.chromium.org/luci/cipd/api/cipd/v1/repopb"
)

// GetACLs returns ACLs in the metadata as a map: role -> set of principals.
func GetACLs(m *repopb.PrefixMetadata) map[repopb.Role]stringset.Set {
	out := map[repopb.Role]stringset.Set{}
	for _, acl := range m.Acls {
		if len(acl.Principals) == 0 {
			continue
		}
		set := out[acl.Role]
		if set == nil {
			set = stringset.New(len(acl.Principals))
			out[acl.Role] = set
		}
		for _, p := range acl.Principals {
			set.Add(p)
		}
	}
	return out
}
