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

func TestInvDatastoreIter(t *testing.T) {
	t.Parallel()

	run := func(c context.Context, query *datastore.Query, limit int) ([]*Invocation, error) {
		it := invDatastoreIter{}
		it.start(c, query)
		defer it.stop()
		invs := []*Invocation{}
		for len(invs) != limit {
			switch inv, err := it.next(); {
			case err != nil:
				return nil, err
			case inv == nil:
				return invs, nil // fetched everything we had
			default:
				invs = append(invs, inv)
			}
		}
		return invs, nil
	}

	c := memory.Use(context.Background())

	Convey("Empty", t, func() {
		invs, err := run(c, datastore.NewQuery("Invocation"), 100)
		So(err, ShouldBeNil)
		So(len(invs), ShouldEqual, 0)
	})

	Convey("Not empty", t, func() {
		original := []*Invocation{
			{ID: 1},
			{ID: 2},
			{ID: 3},
			{ID: 4},
			{ID: 5},
		}
		datastore.Put(c, original)
		datastore.GetTestable(c).CatchupIndexes()

		Convey("No limit", func() {
			q := datastore.NewQuery("Invocation").Order("__key__")
			invs, err := run(c, q, 100)
			So(err, ShouldBeNil)
			So(invs, ShouldResemble, original)
		})

		Convey("With limit", func() {
			q := datastore.NewQuery("Invocation").Order("__key__")

			gtq := q
			invs, err := run(c, gtq, 2)
			So(err, ShouldBeNil)
			So(invs, ShouldResemble, original[:2])

			gtq = q.Gt("__key__", datastore.KeyForObj(c, invs[1]))
			invs, err = run(c, gtq, 2)
			So(err, ShouldBeNil)
			So(invs, ShouldResemble, original[2:4])

			gtq = q.Gt("__key__", datastore.KeyForObj(c, invs[1]))
			invs, err = run(c, gtq, 2)
			So(err, ShouldBeNil)
			So(invs, ShouldResemble, original[4:5])

			gtq = q.Gt("__key__", datastore.KeyForObj(c, invs[0]))
			invs, err = run(c, gtq, 2)
			So(err, ShouldBeNil)
			So(invs, ShouldBeEmpty)
		})

		Convey("With error", func() {
			dsErr := fmt.Errorf("boo")

			brokenC, breaker := featureBreaker.FilterRDS(c, nil)
			breaker.BreakFeatures(dsErr, "Run")

			q := datastore.NewQuery("Invocation").Order("__key__")
			invs, err := run(brokenC, q, 100)
			So(err, ShouldEqual, dsErr)
			So(len(invs), ShouldEqual, 0)
		})
	})
}
