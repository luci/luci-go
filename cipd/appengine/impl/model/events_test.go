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

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/service/datastore"

	repopb "go.chromium.org/luci/cipd/api/cipd/v1/repopb"
	"go.chromium.org/luci/cipd/appengine/impl/testutil"
)

func TestEvents(t *testing.T) {
	t.Parallel()

	ftt.Run("With datastore", t, func(t *ftt.Test) {
		ctx, tc, _ := testutil.TestingContext()

		t.Run("Emitting events", func(t *ftt.Test) {
			// First batch.
			ev := Events{}
			ev.Emit(&repopb.Event{
				Kind:    repopb.EventKind_PACKAGE_CREATED,
				Package: "a/b/c",
			})
			ev.Emit(&repopb.Event{
				Kind:     repopb.EventKind_INSTANCE_TAG_ATTACHED,
				Package:  "a/b/c",
				Instance: "cccc",
				Tag:      "k:v",
			})
			assert.Loosely(t, ev.Flush(ctx), should.BeNil)

			// Second batch a bit later.
			tc.Add(time.Second)
			ev.Emit(&repopb.Event{
				Kind:    repopb.EventKind_INSTANCE_REF_SET,
				Package: "x/y/z",
			})
			ev.Emit(&repopb.Event{
				Kind:    repopb.EventKind_INSTANCE_TAG_ATTACHED,
				Package: "x/y/z",
			})
			assert.Loosely(t, ev.Flush(ctx), should.BeNil)

			// Stored events have Who and When populated too. Ordered by newest to
			// oldest, with events in a single batch separated by a fake nanosecond.
			assert.Loosely(t, GetEvents(ctx), should.Match([]*repopb.Event{
				{
					Kind:    repopb.EventKind_INSTANCE_TAG_ATTACHED,
					Package: "x/y/z",
					Who:     string(testutil.TestUser),
					When:    timestamppb.New(testutil.TestTime.Add(time.Second + 1)),
				},
				{
					Kind:    repopb.EventKind_INSTANCE_REF_SET,
					Package: "x/y/z",
					Who:     string(testutil.TestUser),
					When:    timestamppb.New(testutil.TestTime.Add(time.Second)),
				},
				{
					Kind:     repopb.EventKind_INSTANCE_TAG_ATTACHED,
					Package:  "a/b/c",
					Instance: "cccc",
					Tag:      "k:v",
					Who:      string(testutil.TestUser),
					When:     timestamppb.New(testutil.TestTime.Add(1)),
				},
				{
					Kind:    repopb.EventKind_PACKAGE_CREATED,
					Package: "a/b/c",
					Who:     string(testutil.TestUser),
					When:    timestamppb.New(testutil.TestTime),
				},
			}))
		})
	})
}

func TestEmitMetadataEvents(t *testing.T) {
	t.Parallel()

	ftt.Run("Works", t, func(t *ftt.Test) {
		ctx, _, _ := testutil.TestingContext()

		diffACLs := func(before, after []*repopb.PrefixMetadata_ACL) *repopb.Event {
			datastore.RunInTransaction(ctx, func(ctx context.Context) error {
				assert.Loosely(t, EmitMetadataEvents(ctx,
					&repopb.PrefixMetadata{Prefix: "pfx", Acls: before},
					&repopb.PrefixMetadata{Prefix: "pfx", Acls: after},
				), should.BeNil)
				return nil
			}, nil)
			events := GetEvents(ctx)
			if len(events) == 0 {
				return nil
			}
			assert.Loosely(t, events, should.HaveLength(1))
			return events[0]
		}

		t.Run("Empty", func(t *ftt.Test) {
			assert.Loosely(t, diffACLs(nil, nil), should.BeNil)
		})

		t.Run("Add some", func(t *ftt.Test) {
			ev := diffACLs(nil, []*repopb.PrefixMetadata_ACL{
				{
					Role:       repopb.Role_OWNER,
					Principals: []string{"b", "a"},
				},
				{
					Role:       repopb.Role_READER,
					Principals: []string{"a"},
				},
				{
					Role: repopb.Role_WRITER,
				},
			})
			assert.Loosely(t, ev, should.Match(&repopb.Event{
				Kind:    repopb.EventKind_PREFIX_ACL_CHANGED,
				Package: "pfx",
				Who:     string(testutil.TestUser),
				When:    timestamppb.New(testutil.TestTime),
				GrantedRole: []*repopb.PrefixMetadata_ACL{
					{
						Role:       repopb.Role_READER,
						Principals: []string{"a"},
					},
					{
						Role:       repopb.Role_OWNER,
						Principals: []string{"a", "b"}, // sorted here
					},
				},
			}))
		})

		t.Run("Remove some", func(t *ftt.Test) {
			ev := diffACLs([]*repopb.PrefixMetadata_ACL{
				{
					Role:       repopb.Role_OWNER,
					Principals: []string{"b", "a"},
				},
				{
					Role:       repopb.Role_READER,
					Principals: []string{"a"},
				},
				{
					Role: repopb.Role_WRITER,
				},
			}, nil)
			assert.Loosely(t, ev, should.Match(&repopb.Event{
				Kind:    repopb.EventKind_PREFIX_ACL_CHANGED,
				Package: "pfx",
				Who:     string(testutil.TestUser),
				When:    timestamppb.New(testutil.TestTime),
				RevokedRole: []*repopb.PrefixMetadata_ACL{
					{
						Role:       repopb.Role_READER,
						Principals: []string{"a"},
					},
					{
						Role:       repopb.Role_OWNER,
						Principals: []string{"a", "b"}, // sorted here
					},
				},
			}))
		})

		t.Run("Change some", func(t *ftt.Test) {
			ev := diffACLs([]*repopb.PrefixMetadata_ACL{
				{
					Role:       repopb.Role_OWNER,
					Principals: []string{"b", "a", "c"},
				},
			}, []*repopb.PrefixMetadata_ACL{
				{
					Role:       repopb.Role_OWNER,
					Principals: []string{"b", "d", "e"},
				},
			})
			assert.Loosely(t, ev, should.Match(&repopb.Event{
				Kind:    repopb.EventKind_PREFIX_ACL_CHANGED,
				Package: "pfx",
				Who:     string(testutil.TestUser),
				When:    timestamppb.New(testutil.TestTime),
				GrantedRole: []*repopb.PrefixMetadata_ACL{
					{
						Role:       repopb.Role_OWNER,
						Principals: []string{"d", "e"}, // sorted here
					},
				},
				RevokedRole: []*repopb.PrefixMetadata_ACL{
					{
						Role:       repopb.Role_OWNER,
						Principals: []string{"a", "c"}, // sorted here
					},
				},
			}))
		})
	})
}

// GetEvents is a helper for tests, it grabs all events from the datastore,
// ordering them by timestamp (most recent first).
//
// Calls datastore.GetTestable(ctx).CatchupIndexes() inside.
func GetEvents(ctx context.Context) []*repopb.Event {
	datastore.GetTestable(ctx).CatchupIndexes()
	ev, err := QueryEvents(ctx, NewEventsQuery())
	if err != nil {
		panic(err)
	}
	return ev
}
