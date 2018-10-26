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
	"go.chromium.org/luci/cipd/appengine/impl/testutil"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestEvents(t *testing.T) {
	t.Parallel()

	Convey("With datastore", t, func() {
		ctx, tc, _ := testutil.TestingContext()

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
					Who:     string(testutil.TestUser),
					When:    google.NewTimestamp(testutil.TestTime.Add(time.Second + 1)),
				},
				{
					Kind:    api.EventKind_INSTANCE_REF_SET,
					Package: "x/y/z",
					Who:     string(testutil.TestUser),
					When:    google.NewTimestamp(testutil.TestTime.Add(time.Second)),
				},
				{
					Kind:     api.EventKind_INSTANCE_TAG_ATTACHED,
					Package:  "a/b/c",
					Instance: "cccc",
					Tag:      "k:v",
					Who:      string(testutil.TestUser),
					When:     google.NewTimestamp(testutil.TestTime.Add(1)),
				},
				{
					Kind:    api.EventKind_PACKAGE_CREATED,
					Package: "a/b/c",
					Who:     string(testutil.TestUser),
					When:    google.NewTimestamp(testutil.TestTime),
				},
			})
		})
	})
}

func TestEmitMetadataEvents(t *testing.T) {
	t.Parallel()

	Convey("Works", t, func() {
		ctx, _, _ := testutil.TestingContext()

		diffACLs := func(before, after []*api.PrefixMetadata_ACL) *api.Event {
			So(EmitMetadataEvents(ctx,
				&api.PrefixMetadata{Prefix: "pfx", Acls: before},
				&api.PrefixMetadata{Prefix: "pfx", Acls: after},
			), ShouldBeNil)
			events := GetEvents(ctx)
			if len(events) == 0 {
				return nil
			}
			So(events, ShouldHaveLength, 1)
			return events[0]
		}

		Convey("Empty", func() {
			So(diffACLs(nil, nil), ShouldBeNil)
		})

		Convey("Add some", func() {
			ev := diffACLs(nil, []*api.PrefixMetadata_ACL{
				{
					Role:       api.Role_OWNER,
					Principals: []string{"b", "a"},
				},
				{
					Role:       api.Role_READER,
					Principals: []string{"a"},
				},
				{
					Role: api.Role_WRITER,
				},
			})
			So(ev, ShouldResembleProto, &api.Event{
				Kind:    api.EventKind_PREFIX_ACL_CHANGED,
				Package: "pfx",
				Who:     string(testutil.TestUser),
				When:    google.NewTimestamp(testutil.TestTime),
				GrantedRole: []*api.PrefixMetadata_ACL{
					{
						Role:       api.Role_READER,
						Principals: []string{"a"},
					},
					{
						Role:       api.Role_OWNER,
						Principals: []string{"a", "b"}, // sorted here
					},
				},
			})
		})

		Convey("Remove some", func() {
			ev := diffACLs([]*api.PrefixMetadata_ACL{
				{
					Role:       api.Role_OWNER,
					Principals: []string{"b", "a"},
				},
				{
					Role:       api.Role_READER,
					Principals: []string{"a"},
				},
				{
					Role: api.Role_WRITER,
				},
			}, nil)
			So(ev, ShouldResembleProto, &api.Event{
				Kind:    api.EventKind_PREFIX_ACL_CHANGED,
				Package: "pfx",
				Who:     string(testutil.TestUser),
				When:    google.NewTimestamp(testutil.TestTime),
				RevokedRole: []*api.PrefixMetadata_ACL{
					{
						Role:       api.Role_READER,
						Principals: []string{"a"},
					},
					{
						Role:       api.Role_OWNER,
						Principals: []string{"a", "b"}, // sorted here
					},
				},
			})
		})

		Convey("Change some", func() {
			ev := diffACLs([]*api.PrefixMetadata_ACL{
				{
					Role:       api.Role_OWNER,
					Principals: []string{"b", "a", "c"},
				},
			}, []*api.PrefixMetadata_ACL{
				{
					Role:       api.Role_OWNER,
					Principals: []string{"b", "d", "e"},
				},
			})
			So(ev, ShouldResembleProto, &api.Event{
				Kind:    api.EventKind_PREFIX_ACL_CHANGED,
				Package: "pfx",
				Who:     string(testutil.TestUser),
				When:    google.NewTimestamp(testutil.TestTime),
				GrantedRole: []*api.PrefixMetadata_ACL{
					{
						Role:       api.Role_OWNER,
						Principals: []string{"d", "e"}, // sorted here
					},
				},
				RevokedRole: []*api.PrefixMetadata_ACL{
					{
						Role:       api.Role_OWNER,
						Principals: []string{"a", "c"}, // sorted here
					},
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
