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
	"testing"
	"time"

	"golang.org/x/net/context"

	"go.chromium.org/luci/common/clock/testclock"

	. "github.com/smartystreets/goconvey/convey"
)

func TestDBCache(t *testing.T) {
	Convey("DBCache works", t, func() {
		c := context.Background()
		c, tc := testclock.UseTime(c, time.Unix(1442540000, 0))

		factory := NewDBCache(func(c context.Context, prev DB) (DB, error) {
			if prev == nil {
				return &SnapshotDB{Rev: 1}, nil
			}
			cpy := *prev.(*SnapshotDB)
			cpy.Rev++
			return &cpy, nil
		})

		// Initial fetch.
		db, err := factory(c)
		So(err, ShouldBeNil)
		So(db.(*SnapshotDB).Rev, ShouldEqual, 1)

		// Refetch, using cached copy.
		db, err = factory(c)
		So(err, ShouldBeNil)
		So(db.(*SnapshotDB).Rev, ShouldEqual, 1)

		// Advance time to expire cache.
		tc.Add(11 * time.Second)

		// Returns new copy now.
		db, err = factory(c)
		So(err, ShouldBeNil)
		So(db.(*SnapshotDB).Rev, ShouldEqual, 2)
	})
}
