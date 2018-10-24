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

package model

import (
	"context"
	"testing"
	"time"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/common/proto/google"

	api "go.chromium.org/luci/cipd/api/cipd/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestEvents(t *testing.T) {
	t.Parallel()

	Convey("With datastore", t, func() {
		ctx, tc, _ := TestingContext()

		Convey("Emitting events", func() {
			// First batch.
			ev := Events{}
			ev.Emit(&api.Event{
				Kind:    api.EventKind_PACKAGE_CREATED,
				Package: "a/b/c",
			})
			ev.Emit(&api.Event{
				Kind:     api.EventKind_INSTANCE_TAG_ATTACHED,
				Package:  "a/b/c",
				Instance: "cccc",
				Tag:      "k:v",
			})
			So(ev.Flush(ctx), ShouldBeNil)

			// Second batch a bit later.
			tc.Add(time.Second)
			ev.Emit(&api.Event{
				Kind:    api.EventKind_INSTANCE_REF_SET,
				Package: "x/y/z",
			})
			ev.Emit(&api.Event{
				Kind:    api.EventKind_INSTANCE_TAG_ATTACHED,
				Package: "x/y/z",
			})
			So(ev.Flush(ctx), ShouldBeNil)

			// Stored events have Who and When populated too. Ordered by newest to
			// oldest, with events in a single batch separated by a fake nanosecond.
			So(GetEvents(ctx), ShouldResembleProto, []*api.Event{
				{
					Kind:    api.EventKind_INSTANCE_TAG_ATTACHED,
					Package: "x/y/z",
					Who:     string(testUser),
					When:    google.NewTimestamp(testTime.Add(time.Second + 1)),
				},
				{
					Kind:    api.EventKind_INSTANCE_REF_SET,
					Package: "x/y/z",
					Who:     string(testUser),
					When:    google.NewTimestamp(testTime.Add(time.Second)),
				},
				{
					Kind:     api.EventKind_INSTANCE_TAG_ATTACHED,
					Package:  "a/b/c",
					Instance: "cccc",
					Tag:      "k:v",
					Who:      string(testUser),
					When:     google.NewTimestamp(testTime.Add(1)),
				},
				{
					Kind:    api.EventKind_PACKAGE_CREATED,
					Package: "a/b/c",
					Who:     string(testUser),
					When:    google.NewTimestamp(testTime),
				},
			})
		})
	})
}

// GetEvents is a helper for tests, it grabs all events from the datastore,
// ordering them by timestamp (most recent first).
//
// Calls datastore.GetTestable(ctx).CatchupIndexes() inside.
func GetEvents(c context.Context) []*api.Event {
	datastore.GetTestable(c).CatchupIndexes()
	ev, err := QueryEvents(c, NewEventsQuery())
	if err != nil {
		panic(err)
	}
	return ev
}
