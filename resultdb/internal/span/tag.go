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

package span

import (
	"go.chromium.org/luci/resultdb/pbutil"
	typepb "go.chromium.org/luci/resultdb/proto/type"
)

// TagRowID returns a value for the TagId column in InvocationsByTag table.
func TagRowID(tag *typepb.StringPair) string {
	return prefixWithHash(pbutil.StringPairToString(tag))
}

// TagRowIDs converts tags to tag row IDs. See also TagRowID.
func TagRowIDs(tags ...*typepb.StringPair) []string {
	ids := make([]string, len(tags))
	for i, t := range tags {
		ids[i] = TagRowID(t)
	}
	return ids
}
