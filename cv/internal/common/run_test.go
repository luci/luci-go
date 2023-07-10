// Copyright 2020 The LUCI Authors.
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

package common

import (
	"sort"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestID(t *testing.T) {
	t.Parallel()

	Convey("ID works", t, func() {
		id := MakeRunID("infra", endOfTheWorld.Add(-time.Minute), 1, []byte{65, 15})
		So(id, ShouldEqual, RunID("infra/0000000060000-1-410f"))
		// Assert separately to ensure # of digits doesn't change,
		// as this will break sorting order with IDs makde before.
		So(id.InverseTS(), ShouldHaveLength, 13)
		So(id.InverseTS(), ShouldResemble, "0000000060000")
		So(id.LUCIProject(), ShouldResemble, "infra")
		So(id.AttemptKey(), ShouldResemble, "410f")

		Convey("lexical ordering is oldest last", func() {
			earlierId := MakeRunID("infra", endOfTheWorld.Add(-time.Hour), 2, []byte{31, 44})
			So(earlierId, ShouldEqual, RunID("infra/0000003600000-2-1f2c"))
			So(earlierId, ShouldBeGreaterThan, id)
		})

		Convey("works for recent date", func() {
			earlierId := MakeRunID("infra", time.Date(2020, 01, 01, 1, 1, 1, 2, time.UTC), 1, []byte{31, 44})
			So(earlierId, ShouldEqual, RunID("infra/9045130335854-1-1f2c"))
			So(earlierId, ShouldBeGreaterThan, id)
		})

		Convey("panics if computers survive after endOfTheWorld", func() {
			So(func() {
				MakeRunID("infra", endOfTheWorld.Add(time.Millisecond), 1, []byte{31, 44})
			}, ShouldPanic)
		})

		Convey("panics if time machine is invented", func() {
			// Load Hill Valley's presumed timezone.
			la, err := time.LoadLocation("America/Los_Angeles")
			So(err, ShouldBeNil)
			So(func() {
				MakeRunID("infra", time.Date(1955, time.November, 5, 6, 15, 0, 0, la), 1, []byte{31, 44})
			}, ShouldPanic)
		})

		Convey("PublicID", func() {
			publicID := id.PublicID()
			So(publicID, ShouldEqual, "projects/infra/runs/0000000060000-1-410f")
			id2, err := FromPublicRunID(publicID)
			So(err, ShouldBeNil)
			So(id2, ShouldResemble, id)

			Convey("panics if ID is invalid", func() {
				So(func() { RunID("something good").PublicID() }, ShouldPanic)
			})

			Convey("errors if Public ID is invalid", func() {
				_, err := FromPublicRunID("0000000060000-1-410f")
				So(err, ShouldErrLike, "must be in the form")
				_, err = FromPublicRunID("infra/0000000060000-1-410f")
				So(err, ShouldErrLike, "must be in the form")
				_, err = FromPublicRunID("pRoJeCtS/infra/runs/0000000060000-1-410f")
				So(err, ShouldErrLike, "must be in the form")
				_, err = FromPublicRunID("projects/infra/RuNs/0000000060000-1-410f")
				So(err, ShouldErrLike, "must be in the form")
			})
		})

		Convey("Vallidate", func() {
			id := MakeRunID("infra", time.Date(2020, 01, 01, 1, 1, 1, 2, time.UTC), 1, []byte{31, 44})
			So(id.Validate(), ShouldBeNil)
			minimal := RunID("i/1-1-a")
			So(minimal.Validate(), ShouldBeNil)

			So(RunID("i/1-1-").Validate(), ShouldErrLike, "digest")
			So(RunID("i/1-1").Validate(), ShouldErrLike, "version")
			So(RunID("i/1").Validate(), ShouldErrLike, "InverseTS")
			So(RunID("/1-1-a").Validate(), ShouldErrLike, "project")

			So(RunID("+/1-1-a").Validate(), ShouldErrLike, "invalid character at 0 (+)")
			So(RunID("a/a-1-a").Validate(), ShouldErrLike, "InverseTS")
			So(RunID("a/1-b-a").Validate(), ShouldErrLike, "version")
		})
	})
}

func TestIDs(t *testing.T) {
	t.Parallel()

	Convey("IDs WithoutSorted works", t, func() {
		So(MakeRunIDs().WithoutSorted(MakeRunIDs("1")), ShouldResemble, MakeRunIDs())
		So(RunIDs(nil).WithoutSorted(MakeRunIDs("1")), ShouldBeNil)

		ids := MakeRunIDs("5", "8", "2")
		sort.Sort(ids)
		So(ids, ShouldResemble, MakeRunIDs("2", "5", "8"))

		So(ids.Equal(MakeRunIDs("2", "5", "8")), ShouldBeTrue)
		So(ids.Equal(MakeRunIDs("2", "5", "8", "8")), ShouldBeFalse)

		assertSameSlice(ids.WithoutSorted(nil), ids)
		So(ids, ShouldResemble, MakeRunIDs("2", "5", "8"))

		assertSameSlice(ids.WithoutSorted(MakeRunIDs("1", "3", "9")), ids)
		So(ids, ShouldResemble, MakeRunIDs("2", "5", "8"))

		So(ids.WithoutSorted(MakeRunIDs("1", "5", "9")), ShouldResemble, MakeRunIDs("2", "8"))
		So(ids, ShouldResemble, MakeRunIDs("2", "5", "8"))

		So(ids.WithoutSorted(MakeRunIDs("1", "5", "5", "7")), ShouldResemble, MakeRunIDs("2", "8"))
		So(ids, ShouldResemble, MakeRunIDs("2", "5", "8"))
	})

	Convey("IDs InsertSorted & ContainsSorted works", t, func() {
		ids := MakeRunIDs()

		So(ids.ContainsSorted(RunID("5")), ShouldBeFalse)
		ids.InsertSorted(RunID("5"))
		So(ids, ShouldResemble, MakeRunIDs("5"))
		So(ids.ContainsSorted(RunID("5")), ShouldBeTrue)

		So(ids.ContainsSorted(RunID("2")), ShouldBeFalse)
		ids.InsertSorted(RunID("2"))
		So(ids, ShouldResemble, MakeRunIDs("2", "5"))
		So(ids.ContainsSorted(RunID("2")), ShouldBeTrue)

		So(ids.ContainsSorted(RunID("3")), ShouldBeFalse)
		ids.InsertSorted(RunID("3"))
		So(ids, ShouldResemble, MakeRunIDs("2", "3", "5"))
		So(ids.ContainsSorted(RunID("3")), ShouldBeTrue)
	})

	Convey("IDs DelSorted works", t, func() {
		ids := MakeRunIDs()
		So(ids.DelSorted(RunID("1")), ShouldBeFalse)

		ids = MakeRunIDs("2", "3", "5")
		So(ids.DelSorted(RunID("1")), ShouldBeFalse)
		So(ids.DelSorted(RunID("10")), ShouldBeFalse)
		So(ids.DelSorted(RunID("3")), ShouldBeTrue)
		So(ids, ShouldResemble, MakeRunIDs("2", "5"))
		So(ids.DelSorted(RunID("5")), ShouldBeTrue)
		So(ids, ShouldResemble, MakeRunIDs("2"))
		So(ids.DelSorted(RunID("2")), ShouldBeTrue)
		So(ids, ShouldResemble, MakeRunIDs())
	})

	Convey("IDs DifferenceSorted works", t, func() {
		ids := MakeRunIDs()
		So(ids.DifferenceSorted(MakeRunIDs("1")), ShouldBeEmpty)

		ids = MakeRunIDs("2", "3", "5")
		So(ids.DifferenceSorted(nil), ShouldResemble, MakeRunIDs("2", "3", "5"))
		So(ids.DifferenceSorted(MakeRunIDs()), ShouldResemble, MakeRunIDs("2", "3", "5"))
		So(ids.DifferenceSorted(MakeRunIDs("3")), ShouldResemble, MakeRunIDs("2", "5"))
		So(ids.DifferenceSorted(MakeRunIDs("4")), ShouldResemble, MakeRunIDs("2", "3", "5"))
		So(ids.DifferenceSorted(MakeRunIDs("1", "3", "4")), ShouldResemble, MakeRunIDs("2", "5"))
		So(ids.DifferenceSorted(MakeRunIDs("1", "2", "3", "4", "5")), ShouldBeEmpty)
	})
}

func assertSameSlice(a, b RunIDs) {
	// Go doesn't allow comparing slices, so compare their contents and ensure
	// pointers to the first element are the same.
	So(a, ShouldResemble, b)
	So(&a[0], ShouldEqual, &b[0])
}
