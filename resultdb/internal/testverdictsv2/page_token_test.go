// Copyright 2026 The LUCI Authors.
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

package testverdictsv2

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/resultdb/internal/pagination"
	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/testresultsv2"
)

func TestPageToken(t *testing.T) {
	ftt.Run("PageToken", t, func(t *ftt.Test) {
		t.Run("Roundtrips", func(t *ftt.Test) {
			token := PageToken{
				UIPriority: 50,
				ID: testresultsv2.VerdictID{
					RootInvocationShardID: rootinvocations.ShardID{
						RootInvocationID: "inv",
						ShardIndex:       1,
					},
					ModuleName:        "module",
					ModuleScheme:      "scheme",
					ModuleVariantHash: "hash",
					CoarseName:        "coarse",
					FineName:          "fine",
					CaseName:          "case",
				},
				RequestOrdinal: 2,
			}
			serialized := token.Serialize()
			parsed, err := ParsePageToken(serialized)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, parsed, should.Match(token))
		})

		t.Run("Empty", func(t *ftt.Test) {
			token := PageToken{}
			serialized := token.Serialize()
			assert.Loosely(t, serialized, should.BeEmpty)

			parsed, err := ParsePageToken("")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, parsed, should.Match(token))
		})

		t.Run("Parse errors", func(t *ftt.Test) {
			t.Run("Invalid token", func(t *ftt.Test) {
				_, err := ParsePageToken("blah")
				assert.Loosely(t, err, should.ErrLike("parse: "))
			})
			t.Run("Invalid number of parts", func(t *ftt.Test) {
				// Create a token with fewer than 10 parts. This error might only be
				// expected if a user passes a pagination token from the wrong RPC back.
				token := pagination.Token("1", "2")
				_, err := ParsePageToken(token)
				assert.Loosely(t, err, should.ErrLike("expected 10 components"))
			})
		})
	})
}
