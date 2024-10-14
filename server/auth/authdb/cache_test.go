// Copyright 2016 The LUCI Authors.
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

package authdb

import (
	"context"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestDBCache(t *testing.T) {
	ftt.Run("DBCache works", t, func(t *ftt.Test) {
		c := context.Background()
		c, tc := testclock.UseTime(c, time.Unix(1442540000, 0))

		factory := NewDBCache(func(ctx context.Context, prev DB) (DB, error) {
			if prev == nil {
				return &SnapshotDB{Rev: 1}, nil
			}
			cpy := *prev.(*SnapshotDB)
			cpy.Rev++
			return &cpy, nil
		})

		// Initial fetch.
		db, err := factory(c)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, db.(*SnapshotDB).Rev, should.Equal(1))

		// Refetch, using cached copy.
		db, err = factory(c)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, db.(*SnapshotDB).Rev, should.Equal(1))

		// Advance time to expire cache.
		tc.Add(11 * time.Second)

		// Returns new copy now.
		db, err = factory(c)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, db.(*SnapshotDB).Rev, should.Equal(2))
	})
}
