// Copyright 2015 The LUCI Authors.
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

package bigtable

import (
	"fmt"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestRowKey(t *testing.T) {
	t.Parallel()

	ftt.Run(`A row key, constructed from "test-project" and "a/b/+/c/d"`, t, func(t *ftt.Test) {
		project := "test-project"
		path := "a/b/+/c/d"

		rk := newRowKey(project, path, 1337, 42)

		t.Run(`Shares a path with a row key from the same Path.`, func(t *ftt.Test) {
			assert.Loosely(t, rk.sharesPathWith(newRowKey(project, path, 2468, 0)), should.BeTrue)
		})

		for _, project := range []string{
			"",
			"other-test-project",
		} {
			for _, path := range []string{
				"a/b/+/c",
				"asdf",
				"",
			} {
				t.Run(fmt.Sprintf(`Does not share a path with project %q, path %q`, project, path), func(t *ftt.Test) {
					assert.Loosely(t, rk.sharesPathWith(newRowKey(project, path, 0, 0)), should.BeFalse)
				})
			}
		}

		t.Run(`Can be encoded, then decoded into its fields.`, func(t *ftt.Test) {
			enc := rk.encode()
			assert.Loosely(t, len(enc), should.BeLessThanOrEqual(maxEncodedKeySize))

			drk, err := decodeRowKey(enc)
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, drk.pathHash, should.Resemble(rk.pathHash))
			assert.Loosely(t, drk.index, should.Equal(rk.index))
			assert.Loosely(t, drk.count, should.Equal(rk.count))
		})
	})

	ftt.Run(`A series of ordered row keys`, t, func(t *ftt.Test) {
		prev := ""
		for _, i := range []int64{
			-1, /* Why not? */
			0,
			7,
			8,
			257,
			1029,
			1337,
		} {
			t.Run(fmt.Sprintf(`Row key %d should be ascendingly sorted and parsable.`, i), func(t *ftt.Test) {
				rk := newRowKey("test-project", "test", i, i)

				// Test that it encodes/decodes back to identity.
				enc := rk.encode()
				drk, err := decodeRowKey(enc)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, drk.index, should.Equal(i))

				// Assert that it is ordered.
				if prev != "" {
					assert.Loosely(t, prev, should.BeLessThan(enc))

					prevp, err := decodeRowKey(prev)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, prevp.sharesPathWith(rk), should.BeTrue)
					assert.Loosely(t, prevp.index, should.BeLessThan(drk.index))
					assert.Loosely(t, prevp.count, should.BeLessThan(drk.count))
				}
			})
		}
	})

	ftt.Run(`Invalid row keys will fail to decode with "errMalformedRowKey".`, t, func(t *ftt.Test) {
		for _, tc := range []struct {
			name string
			v    string
		}{
			{"No tilde", "a94a8fe5ccb19ba61c4c0873d391e987982fbbd38080"},
			{"No path hash", "~8080"},
			{"Bad hex path hash", "badhex~8080"},
			{"Path has too short", "4a54700540127~8080"},
			{"Bad hex index", "a94a8fe5ccb19ba61c4c0873d391e987982fbbd3~badhex"},
			{"Missing index.", "a94a8fe5ccb19ba61c4c0873d391e987982fbbd3~"},
			{"Varint overflow", "a94a8fe5ccb19ba61c4c0873d391e987982fbbd3~ffffffffffff"},
			{"Trailing data", "a94a8fe5ccb19ba61c4c0873d391e987982fbbd3~8080badd06"},
		} {
			t.Run(fmt.Sprintf(`Row key fails to decode [%s]: %q`, tc.name, tc.v), func(t *ftt.Test) {
				_, err := decodeRowKey(tc.v)
				assert.Loosely(t, err, should.Equal(errMalformedRowKey))
			})
		}
	})
}
