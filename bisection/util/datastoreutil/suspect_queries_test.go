// Copyright 2022 The LUCI Authors.
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

package datastoreutil

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"

	"go.chromium.org/luci/bisection/model"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
)

func TestCountLatestRevertsCreated(t *testing.T) {
	t.Parallel()
	c := memory.Use(context.Background())

	datastore.GetTestable(c).AddIndexes(
		&datastore.IndexDefinition{
			Kind: "Suspect",
			SortBy: []datastore.IndexColumn{
				{
					Property: "is_revert_created",
				},
				{
					Property: "revert_create_time",
				},
			},
		},
	)
	datastore.GetTestable(c).CatchupIndexes()

	// Set test clock
	cl := testclock.New(testclock.TestTimeUTC)
	c = clock.Set(c, cl)

	Convey("No suspects at all", t, func() {
		count, err := CountLatestRevertsCreated(c, 24)
		So(err, ShouldBeNil)
		So(count, ShouldEqual, 0)
	})

	Convey("Count of recent created reverts", t, func() {
		// Set up suspects
		suspect1 := &model.Suspect{}
		suspect2 := &model.Suspect{
			RevertDetails: model.RevertDetails{
				RevertURL:        "https://chromium-test-review.googlesource.com/c/test/project/+/100000",
				IsRevertCreated:  true,
				RevertCreateTime: clock.Now(c),
			},
		}
		suspect3 := &model.Suspect{
			RevertDetails: model.RevertDetails{
				RevertURL:        "https://chromium-test-review.googlesource.com/c/test/project/+/100001",
				IsRevertCreated:  true,
				RevertCreateTime: clock.Now(c).Add(-time.Hour * 72),
			},
		}
		suspect4 := &model.Suspect{
			RevertDetails: model.RevertDetails{
				RevertURL:        "https://chromium-test-review.googlesource.com/c/test/project/+/100002",
				IsRevertCreated:  true,
				RevertCreateTime: clock.Now(c).Add(-time.Hour * 4),
			},
		}
		suspect5 := &model.Suspect{
			RevertDetails: model.RevertDetails{
				RevertURL:        "https://chromium-test-review.googlesource.com/c/test/project/+/100003",
				IsRevertCreated:  true,
				RevertCreateTime: clock.Now(c).Add(-time.Minute * 10),
			},
		}
		suspect6 := &model.Suspect{
			RevertDetails: model.RevertDetails{
				RevertURL:        "https://chromium-test-review.googlesource.com/c/test/project/+/100004",
				IsRevertCreated:  true,
				RevertCreateTime: clock.Now(c).Add(-time.Hour * 24),
			},
		}
		suspect7 := &model.Suspect{
			RevertDetails: model.RevertDetails{
				RevertURL:        "",
				IsRevertCreated:  false,
				RevertCreateTime: clock.Now(c).Add(-time.Minute * 10),
			},
		}
		So(datastore.Put(c, suspect1), ShouldBeNil)
		So(datastore.Put(c, suspect2), ShouldBeNil)
		So(datastore.Put(c, suspect3), ShouldBeNil)
		So(datastore.Put(c, suspect4), ShouldBeNil)
		So(datastore.Put(c, suspect5), ShouldBeNil)
		So(datastore.Put(c, suspect6), ShouldBeNil)
		So(datastore.Put(c, suspect7), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()

		count, err := CountLatestRevertsCreated(c, 24)
		So(err, ShouldBeNil)
		So(count, ShouldEqual, 3)
	})
}

func TestCountLatestRevertsCommitted(t *testing.T) {
	t.Parallel()
	c := memory.Use(context.Background())

	datastore.GetTestable(c).AddIndexes(
		&datastore.IndexDefinition{
			Kind: "Suspect",
			SortBy: []datastore.IndexColumn{
				{
					Property: "is_revert_committed",
				},
				{
					Property: "revert_commit_time",
				},
			},
		},
	)
	datastore.GetTestable(c).CatchupIndexes()

	// Set test clock
	cl := testclock.New(testclock.TestTimeUTC)
	c = clock.Set(c, cl)

	Convey("No suspects at all", t, func() {
		count, err := CountLatestRevertsCommitted(c, 24)
		So(err, ShouldBeNil)
		So(count, ShouldEqual, 0)
	})

	Convey("Count of recent committed reverts", t, func() {
		// Set up suspects
		suspect1 := &model.Suspect{}
		suspect2 := &model.Suspect{
			RevertDetails: model.RevertDetails{
				IsRevertCommitted: true,
				RevertCommitTime:  clock.Now(c),
			},
		}
		suspect3 := &model.Suspect{
			RevertDetails: model.RevertDetails{
				IsRevertCommitted: true,
				RevertCommitTime:  clock.Now(c).Add(-time.Hour * 72),
			},
		}
		suspect4 := &model.Suspect{
			RevertDetails: model.RevertDetails{
				IsRevertCommitted: true,
				RevertCommitTime:  clock.Now(c).Add(-time.Hour * 4),
			},
		}
		suspect5 := &model.Suspect{
			RevertDetails: model.RevertDetails{
				IsRevertCommitted: true,
				RevertCommitTime:  clock.Now(c).Add(-time.Minute * 10),
			},
		}
		suspect6 := &model.Suspect{
			RevertDetails: model.RevertDetails{
				IsRevertCommitted: true,
				RevertCommitTime:  clock.Now(c).Add(-time.Hour * 24),
			},
		}
		suspect7 := &model.Suspect{
			RevertDetails: model.RevertDetails{
				IsRevertCommitted: false,
				RevertCommitTime:  clock.Now(c).Add(-time.Minute * 10),
			},
		}
		So(datastore.Put(c, suspect1), ShouldBeNil)
		So(datastore.Put(c, suspect2), ShouldBeNil)
		So(datastore.Put(c, suspect3), ShouldBeNil)
		So(datastore.Put(c, suspect4), ShouldBeNil)
		So(datastore.Put(c, suspect5), ShouldBeNil)
		So(datastore.Put(c, suspect6), ShouldBeNil)
		So(datastore.Put(c, suspect7), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()

		count, err := CountLatestRevertsCommitted(c, 24)
		So(err, ShouldBeNil)
		So(count, ShouldEqual, 3)
	})
}
