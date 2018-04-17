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

package engine

import (
	"fmt"
	"testing"

	"golang.org/x/net/context"

	"go.chromium.org/gae/filter/featureBreaker"
	"go.chromium.org/gae/impl/memory"
	"go.chromium.org/gae/service/datastore"

	. "github.com/smartystreets/goconvey/convey"
)

func TestInvDatastoreQuery(t *testing.T) {
	t.Parallel()

	run := func(c context.Context, query *datastore.Query, limit int) ([]*Invocation, datastore.Cursor, error) {
		q := invDatastoreQuery{}
		q.start(c, query)
		defer q.stop()
		invs := []*Invocation{}
		for {
			switch inv, err := q.next(); {
			case err != nil:
				return nil, nil, err
			case inv == nil:
				return invs, nil, nil
			case inv != nil:
				invs = append(invs, inv)
			}
			if len(invs) == limit {
				cur, err := q.stop()
				if err != nil {
					return nil, nil, err
				}
				return invs, cur, nil
			}
		}
	}

	c := memory.Use(context.Background())

	Convey("Empty", t, func() {
		invs, cursor, err := run(c, datastore.NewQuery("Invocation"), 100)
		So(err, ShouldBeNil)
		So(len(invs), ShouldEqual, 0)
		So(cursor, ShouldBeNil)
	})

	Convey("Not empty, no pagination", t, func() {
		original := []*Invocation{
			{ID: 1},
			{ID: 2},
			{ID: 3},
			{ID: 4},
			{ID: 5},
		}
		datastore.Put(c, original)
		datastore.GetTestable(c).CatchupIndexes()

		Convey("No pagination", func() {
			q := datastore.NewQuery("Invocation").Order("__key__")
			invs, cursor, err := run(c, q, 100)
			So(err, ShouldBeNil)
			So(invs, ShouldResemble, original)
			So(cursor, ShouldBeNil)
		})

		Convey("With pagination", func() {
			q := datastore.NewQuery("Invocation").Order("__key__")
			var batches [][]*Invocation
			var cursor datastore.Cursor
			for {
				var invs []*Invocation
				var err error
				invs, cursor, err = run(c, q.Start(cursor), 2)
				So(err, ShouldBeNil)
				batches = append(batches, invs)
				if cursor == nil {
					break
				}
			}
			So(batches, ShouldResemble, [][]*Invocation{
				original[:2],
				original[2:4],
				original[4:5],
			})
		})

		Convey("With error", func() {
			dsErr := fmt.Errorf("boo")

			brokenC, breaker := featureBreaker.FilterRDS(c, nil)
			breaker.BreakFeatures(dsErr, "Run")

			q := datastore.NewQuery("Invocation").Order("__key__")
			invs, cursor, err := run(brokenC, q, 100)
			So(err, ShouldEqual, dsErr)
			So(len(invs), ShouldEqual, 0)
			So(cursor, ShouldBeNil)
		})
	})
}
