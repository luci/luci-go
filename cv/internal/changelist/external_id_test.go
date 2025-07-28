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

package changelist

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestExternalID(t *testing.T) {
	t.Parallel()

	ftt.Run("ExternalID works", t, func(t *ftt.Test) {
		t.Run("GobID", func(t *ftt.Test) {
			eid, err := GobID("x-review.example.com", 12)
			assert.NoErr(t, err)
			assert.That(t, eid, should.Match(ExternalID("gerrit/x-review.example.com/12")))

			host, change, err := eid.ParseGobID()
			assert.NoErr(t, err)
			assert.That(t, host, should.Match("x-review.example.com"))
			assert.Loosely(t, change, should.Equal(12))

			assert.That(t, eid.MustURL(), should.Match("https://x-review.example.com/c/12"))
		})

		t.Run("Invalid GobID", func(t *ftt.Test) {
			_, _, err := ExternalID("meh").ParseGobID()
			assert.ErrIsLike(t, err, "is not a valid GobID")

			_, _, err = ExternalID("gerrit/x/y").ParseGobID()
			assert.ErrIsLike(t, err, "is not a valid GobID")
		})
	})
}

func TestJoinExternalURLs(t *testing.T) {
	t.Parallel()

	ftt.Run("JoinExternalURLs works", t, func(t *ftt.Test) {
		gob := func(num int64) ExternalID {
			eid, err := GobID("example.com", num)
			assert.NoErr(t, err)
			return eid
		}
		var eids []ExternalID
		t.Run("with empty input", func(t *ftt.Test) {
			assert.Loosely(t, JoinExternalURLs(eids, ", "), should.BeEmpty)
		})
		t.Run("with single input", func(t *ftt.Test) {
			eids = append(eids, gob(1))
			assert.Loosely(t, JoinExternalURLs(eids, ", "), should.Equal("https://example.com/c/1"))
		})
		t.Run("with > 1 inputs", func(t *ftt.Test) {
			eids = append(eids, gob(1), gob(2))
			assert.Loosely(t, JoinExternalURLs(eids, ", "), should.Equal(
				"https://example.com/c/1, https://example.com/c/2"))
		})
	})
}
