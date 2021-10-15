// Copyright 2021 The LUCI Authors.
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

// Package metadata can extract metadata from Gerrit CLs.
package metadata

import (
	"sort"

	"go.chromium.org/luci/common/git/footer"

	"go.chromium.org/luci/cv/internal/changelist"
)

// Extract extracts CL metadata from Gerrit CL description.
//
// Returns key-value pairs sorted by key first, while values are ordered from
// bottom to top of the CL description.
func Extract(clDescription string) []*changelist.StringPair {
	kvs := footer.ParseMessage(clDescription)
	for k, vs := range footer.ParseLegacyMetadata(clDescription) {
		kvs[k] = append(kvs[k], vs...)
	}
	keys := make([]string, len(kvs))
	for k := range kvs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	// Each key has at least one value.
	out := make([]*changelist.StringPair, 0, len(kvs))
	for _, k := range keys {
		for _, v := range kvs[k] {
			out = append(out, &changelist.StringPair{Key: k, Value: v})
		}
	}
	return out
}
