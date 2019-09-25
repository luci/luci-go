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

package results

import (
	"sort"

	resultspb "go.chromium.org/luci/results/proto/v1"
)

// NormalizeInvocation takes an invocation and orders repeated fields.
func NormalizeInvocation(inv *resultspb.Invocation) {
	for _, test := range inv.Tests {
		for _, variant := range test.Variants {
			for _, result := range variant.Results {
				sortTagsInPlace(result.Tags)
			}
		}

		sort.Slice(test.Variants, func(i, j int) bool {
			return test.Variants[i].VariantId < test.Variants[j].VariantId
		})
	}

	sort.Slice(inv.Tests, func(i, j int) bool {
		return inv.Tests[i].Path < inv.Tests[j].Path
	})

	sortTagsInPlace(inv.Tags)
}

// sortTagsInPlace sorts in-place the tags slice lexicographically by key, then value.
func sortTagsInPlace(tags []*resultspb.StringPair) {
	sort.Slice(tags, func(i, j int) bool {
		if tags[i].Key != tags[j].Key {
			return tags[i].Key < tags[j].Key
		}
		return tags[i].Value < tags[j].Value
	})
}
