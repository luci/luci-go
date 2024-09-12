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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestID(t *testing.T) {
	t.Parallel()

	ftt.Run("ID works", t, func(t *ftt.Test) {
		id := MakeRunID("infra", endOfTheWorld.Add(-time.Minute), 1, []byte{65, 15})
		assert.Loosely(t, id, should.Equal(RunID("infra/0000000060000-1-410f")))
		// Assert separately to ensure # of digits doesn't change,
		// as this will break sorting order with IDs makde before.
		assert.Loosely(t, id.InverseTS(), should.HaveLength(13))
		assert.Loosely(t, id.InverseTS(), should.Match("0000000060000"))
		assert.Loosely(t, id.LUCIProject(), should.Match("infra"))
		assert.Loosely(t, id.AttemptKey(), should.Match("410f"))

		t.Run("lexical ordering is oldest last", func(t *ftt.Test) {
			earlierId := MakeRunID("infra", endOfTheWorld.Add(-time.Hour), 2, []byte{31, 44})
			assert.Loosely(t, earlierId, should.Equal(RunID("infra/0000003600000-2-1f2c")))
			assert.Loosely(t, earlierId, should.BeGreaterThan(id))
		})

		t.Run("works for recent date", func(t *ftt.Test) {
			earlierId := MakeRunID("infra", time.Date(2020, 01, 01, 1, 1, 1, 2, time.UTC), 1, []byte{31, 44})
			assert.Loosely(t, earlierId, should.Equal(RunID("infra/9045130335854-1-1f2c")))
			assert.Loosely(t, earlierId, should.BeGreaterThan(id))
		})

		t.Run("panics if computers survive after endOfTheWorld", func(t *ftt.Test) {
			assert.Loosely(t, func() {
				MakeRunID("infra", endOfTheWorld.Add(time.Millisecond), 1, []byte{31, 44})
			}, should.Panic)
		})

		t.Run("panics if time machine is invented", func(t *ftt.Test) {
			// Load Hill Valley's presumed timezone.
			la, err := time.LoadLocation("America/Los_Angeles")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, func() {
				MakeRunID("infra", time.Date(1955, time.November, 5, 6, 15, 0, 0, la), 1, []byte{31, 44})
			}, should.Panic)
		})

		t.Run("PublicID", func(t *ftt.Test) {
			publicID := id.PublicID()
			assert.Loosely(t, publicID, should.Equal("projects/infra/runs/0000000060000-1-410f"))
			id2, err := FromPublicRunID(publicID)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, id2, should.Resemble(id))

			t.Run("panics if ID is invalid", func(t *ftt.Test) {
				assert.Loosely(t, func() { RunID("something good").PublicID() }, should.Panic)
			})

			t.Run("errors if Public ID is invalid", func(t *ftt.Test) {
				_, err := FromPublicRunID("0000000060000-1-410f")
				assert.Loosely(t, err, should.ErrLike("must be in the form"))
				_, err = FromPublicRunID("infra/0000000060000-1-410f")
				assert.Loosely(t, err, should.ErrLike("must be in the form"))
				_, err = FromPublicRunID("pRoJeCtS/infra/runs/0000000060000-1-410f")
				assert.Loosely(t, err, should.ErrLike("must be in the form"))
				_, err = FromPublicRunID("projects/infra/RuNs/0000000060000-1-410f")
				assert.Loosely(t, err, should.ErrLike("must be in the form"))
			})
		})

		t.Run("Vallidate", func(t *ftt.Test) {
			id := MakeRunID("infra", time.Date(2020, 01, 01, 1, 1, 1, 2, time.UTC), 1, []byte{31, 44})
			assert.Loosely(t, id.Validate(), should.BeNil)
			minimal := RunID("i/1-1-a")
			assert.Loosely(t, minimal.Validate(), should.BeNil)

			assert.Loosely(t, RunID("i/1-1-").Validate(), should.ErrLike("digest"))
			assert.Loosely(t, RunID("i/1-1").Validate(), should.ErrLike("version"))
			assert.Loosely(t, RunID("i/1").Validate(), should.ErrLike("InverseTS"))
			assert.Loosely(t, RunID("/1-1-a").Validate(), should.ErrLike("project"))

			assert.Loosely(t, RunID("+/1-1-a").Validate(), should.ErrLike("invalid character at 0 (+)"))
			assert.Loosely(t, RunID("a/a-1-a").Validate(), should.ErrLike("InverseTS"))
			assert.Loosely(t, RunID("a/1-b-a").Validate(), should.ErrLike("version"))
		})
	})
}

func TestIDs(t *testing.T) {
	t.Parallel()

	ftt.Run("IDs WithoutSorted works", t, func(t *ftt.Test) {
		assert.Loosely(t, MakeRunIDs().WithoutSorted(MakeRunIDs("1")), should.Resemble(MakeRunIDs()))
		assert.Loosely(t, RunIDs(nil).WithoutSorted(MakeRunIDs("1")), should.BeNil)

		ids := MakeRunIDs("5", "8", "2")
		sort.Sort(ids)
		assert.Loosely(t, ids, should.Resemble(MakeRunIDs("2", "5", "8")))

		assert.Loosely(t, ids.Equal(MakeRunIDs("2", "5", "8")), should.BeTrue)
		assert.Loosely(t, ids.Equal(MakeRunIDs("2", "5", "8", "8")), should.BeFalse)

		assertSameSlice(t, ids.WithoutSorted(nil), ids)
		assert.Loosely(t, ids, should.Resemble(MakeRunIDs("2", "5", "8")))

		assertSameSlice(t, ids.WithoutSorted(MakeRunIDs("1", "3", "9")), ids)
		assert.Loosely(t, ids, should.Resemble(MakeRunIDs("2", "5", "8")))

		assert.Loosely(t, ids.WithoutSorted(MakeRunIDs("1", "5", "9")), should.Resemble(MakeRunIDs("2", "8")))
		assert.Loosely(t, ids, should.Resemble(MakeRunIDs("2", "5", "8")))

		assert.Loosely(t, ids.WithoutSorted(MakeRunIDs("1", "5", "5", "7")), should.Resemble(MakeRunIDs("2", "8")))
		assert.Loosely(t, ids, should.Resemble(MakeRunIDs("2", "5", "8")))
	})

	ftt.Run("IDs InsertSorted & ContainsSorted works", t, func(t *ftt.Test) {
		ids := MakeRunIDs()

		assert.Loosely(t, ids.ContainsSorted(RunID("5")), should.BeFalse)
		ids.InsertSorted(RunID("5"))
		assert.Loosely(t, ids, should.Resemble(MakeRunIDs("5")))
		assert.Loosely(t, ids.ContainsSorted(RunID("5")), should.BeTrue)

		assert.Loosely(t, ids.ContainsSorted(RunID("2")), should.BeFalse)
		ids.InsertSorted(RunID("2"))
		assert.Loosely(t, ids, should.Resemble(MakeRunIDs("2", "5")))
		assert.Loosely(t, ids.ContainsSorted(RunID("2")), should.BeTrue)

		assert.Loosely(t, ids.ContainsSorted(RunID("3")), should.BeFalse)
		ids.InsertSorted(RunID("3"))
		assert.Loosely(t, ids, should.Resemble(MakeRunIDs("2", "3", "5")))
		assert.Loosely(t, ids.ContainsSorted(RunID("3")), should.BeTrue)
	})

	ftt.Run("IDs DelSorted works", t, func(t *ftt.Test) {
		ids := MakeRunIDs()
		assert.Loosely(t, ids.DelSorted(RunID("1")), should.BeFalse)

		ids = MakeRunIDs("2", "3", "5")
		assert.Loosely(t, ids.DelSorted(RunID("1")), should.BeFalse)
		assert.Loosely(t, ids.DelSorted(RunID("10")), should.BeFalse)
		assert.Loosely(t, ids.DelSorted(RunID("3")), should.BeTrue)
		assert.Loosely(t, ids, should.Resemble(MakeRunIDs("2", "5")))
		assert.Loosely(t, ids.DelSorted(RunID("5")), should.BeTrue)
		assert.Loosely(t, ids, should.Resemble(MakeRunIDs("2")))
		assert.Loosely(t, ids.DelSorted(RunID("2")), should.BeTrue)
		assert.Loosely(t, ids, should.Resemble(MakeRunIDs()))
	})

	ftt.Run("IDs DifferenceSorted works", t, func(t *ftt.Test) {
		ids := MakeRunIDs()
		assert.Loosely(t, ids.DifferenceSorted(MakeRunIDs("1")), should.BeEmpty)

		ids = MakeRunIDs("2", "3", "5")
		assert.Loosely(t, ids.DifferenceSorted(nil), should.Resemble(MakeRunIDs("2", "3", "5")))
		assert.Loosely(t, ids.DifferenceSorted(MakeRunIDs()), should.Resemble(MakeRunIDs("2", "3", "5")))
		assert.Loosely(t, ids.DifferenceSorted(MakeRunIDs("3")), should.Resemble(MakeRunIDs("2", "5")))
		assert.Loosely(t, ids.DifferenceSorted(MakeRunIDs("4")), should.Resemble(MakeRunIDs("2", "3", "5")))
		assert.Loosely(t, ids.DifferenceSorted(MakeRunIDs("1", "3", "4")), should.Resemble(MakeRunIDs("2", "5")))
		assert.Loosely(t, ids.DifferenceSorted(MakeRunIDs("1", "2", "3", "4", "5")), should.BeEmpty)
	})
}

func assertSameSlice(t testing.TB, a, b RunIDs) {
	t.Helper()

	// Go doesn't allow comparing slices, so compare their contents and ensure
	// pointers to the first element are the same.
	assert.Loosely(t, a, should.Resemble(b), truth.LineContext())
	assert.Loosely(t, &a[0], should.Equal(&b[0]), truth.LineContext())
}
