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
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/codes"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/proto/google"
	"go.chromium.org/luci/grpc/grpcutil"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
	"go.chromium.org/luci/cipd/common"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestTags(t *testing.T) {
	t.Parallel()

	Convey("With datastore", t, func() {
		digest := strings.Repeat("a", 40)

		ctx, tc, as := TestingContext()

		putInst := func(pkg, iid string, pendingProcs []string) *Instance {
			inst := &Instance{
				InstanceID:        iid,
				Package:           PackageKey(ctx, pkg),
				ProcessorsPending: pendingProcs,
			}
			So(datastore.Put(ctx, &Package{Name: pkg}, inst), ShouldBeNil)
			return inst
		}

		tags := func(t ...string) []*api.Tag {
			out := make([]*api.Tag, len(t))
			for i, s := range t {
				out[i] = common.MustParseInstanceTag(s)
			}
			return out
		}

		getTag := func(inst *Instance, tag string) *Tag {
			t := &Tag{
				ID:       TagID(common.MustParseInstanceTag(tag)),
				Instance: datastore.KeyForObj(ctx, inst),
			}
			err := datastore.Get(ctx, t)
			if err == datastore.ErrNoSuchEntity {
				return nil
			}
			So(err, ShouldBeNil)
			return t
		}

		expectedTag := func(inst *Instance, tag string, secs int, who identity.Identity) *Tag {
			return &Tag{
				ID:           TagID(common.MustParseInstanceTag(tag)),
				Instance:     datastore.KeyForObj(ctx, inst),
				Tag:          tag,
				RegisteredBy: string(who),
				RegisteredTs: testTime.Add(time.Duration(secs) * time.Second),
			}
		}

		Convey("Proto() works", func() {
			inst := putInst("pkg", digest, nil)
			t := expectedTag(inst, "k:v", 0, testUser)
			So(t.Proto(), ShouldResembleProto, &api.Tag{
				Key:        "k",
				Value:      "v",
				AttachedBy: string(testUser),
				AttachedTs: google.NewTimestamp(testTime),
			})
		})

		Convey("AttachTags happy paths", func() {
			inst := putInst("pkg", digest, nil)

			// Attach one tag and verify it exist.
			So(AttachTags(ctx, inst, tags("a:0")), ShouldBeNil)
			So(getTag(inst, "a:0"), ShouldResemble, expectedTag(inst, "a:0", 0, testUser))

			// Attach few more at once.
			tc.Add(time.Second)
			So(AttachTags(ctx, inst, tags("a:1", "a:2")), ShouldBeNil)
			So(getTag(inst, "a:1"), ShouldResemble, expectedTag(inst, "a:1", 1, testUser))
			So(getTag(inst, "a:2"), ShouldResemble, expectedTag(inst, "a:2", 1, testUser))

			// Try to reattach an existing one (notice the change in the email),
			// should be ignored.
			So(AttachTags(as("def@example.com"), inst, tags("a:0")), ShouldBeNil)
			So(getTag(inst, "a:0"), ShouldResemble, expectedTag(inst, "a:0", 0, testUser))

			// Try to reattach a bunch of existing ones at once.
			So(AttachTags(as("def@example.com"), inst, tags("a:1", "a:2")), ShouldBeNil)
			So(getTag(inst, "a:1"), ShouldResemble, expectedTag(inst, "a:1", 1, testUser))
			So(getTag(inst, "a:2"), ShouldResemble, expectedTag(inst, "a:2", 1, testUser))

			// Mixed group with new and existing tags.
			tc.Add(time.Second)
			So(AttachTags(as("def@example.com"), inst, tags("a:3", "a:0", "a:4", "a:1")), ShouldBeNil)
			So(getTag(inst, "a:3"), ShouldResemble, expectedTag(inst, "a:3", 2, "user:def@example.com"))
			So(getTag(inst, "a:0"), ShouldResemble, expectedTag(inst, "a:0", 0, testUser))
			So(getTag(inst, "a:4"), ShouldResemble, expectedTag(inst, "a:4", 2, "user:def@example.com"))
			So(getTag(inst, "a:1"), ShouldResemble, expectedTag(inst, "a:1", 1, testUser))

			// All events have been collected.
			So(GetEvents(ctx), ShouldResembleProto, []*api.Event{
				{
					Kind:     api.EventKind_INSTANCE_TAG_ATTACHED,
					Package:  "pkg",
					Instance: digest,
					Tag:      "a:4",
					Who:      "user:def@example.com",
					When:     google.NewTimestamp(testTime.Add(2*time.Second + 1)),
				},
				{
					Kind:     api.EventKind_INSTANCE_TAG_ATTACHED,
					Package:  "pkg",
					Instance: digest,
					Tag:      "a:3",
					Who:      "user:def@example.com",
					When:     google.NewTimestamp(testTime.Add(2 * time.Second)),
				},
				{
					Kind:     api.EventKind_INSTANCE_TAG_ATTACHED,
					Package:  "pkg",
					Instance: digest,
					Tag:      "a:2",
					Who:      string(testUser),
					When:     google.NewTimestamp(testTime.Add(1*time.Second + 1)),
				},
				{
					Kind:     api.EventKind_INSTANCE_TAG_ATTACHED,
					Package:  "pkg",
					Instance: digest,
					Tag:      "a:1",
					Who:      string(testUser),
					When:     google.NewTimestamp(testTime.Add(1 * time.Second)),
				},
				{
					Kind:     api.EventKind_INSTANCE_TAG_ATTACHED,
					Package:  "pkg",
					Instance: digest,
					Tag:      "a:0",
					Who:      string(testUser),
					When:     google.NewTimestamp(testTime),
				},
			})
		})

		Convey("DetachTags happy paths", func() {
			inst := putInst("pkg", digest, nil)

			// Attach a bunch of tags first, so we have something to detach.
			So(AttachTags(ctx, inst, tags("a:0", "a:1", "a:2", "a:3", "a:4")), ShouldBeNil)

			// Detaching one existing.
			tc.Add(time.Second)
			So(getTag(inst, "a:0"), ShouldNotBeNil)
			So(DetachTags(ctx, inst, tags("a:0")), ShouldBeNil)
			So(getTag(inst, "a:0"), ShouldBeNil)

			// Detaching one missing.
			So(DetachTags(ctx, inst, tags("a:z0")), ShouldBeNil)

			// Detaching a bunch of existing.
			tc.Add(time.Second)
			So(getTag(inst, "a:1"), ShouldNotBeNil)
			So(getTag(inst, "a:2"), ShouldNotBeNil)
			So(DetachTags(ctx, inst, tags("a:1", "a:2")), ShouldBeNil)
			So(getTag(inst, "a:1"), ShouldBeNil)
			So(getTag(inst, "a:2"), ShouldBeNil)

			// Detaching a bunch of missing.
			So(DetachTags(ctx, inst, tags("a:z1", "a:z2")), ShouldBeNil)

			// Detaching a mix of existing and missing.
			tc.Add(time.Second)
			So(getTag(inst, "a:3"), ShouldNotBeNil)
			So(getTag(inst, "a:4"), ShouldNotBeNil)
			So(DetachTags(ctx, inst, tags("a:z3", "a:3", "a:z4", "a:4")), ShouldBeNil)
			So(getTag(inst, "a:3"), ShouldBeNil)
			So(getTag(inst, "a:4"), ShouldBeNil)

			// All 'detach' events have been collected (skip checking 'attach' ones).
			So(GetEvents(ctx)[:5], ShouldResembleProto, []*api.Event{
				{
					Kind:     api.EventKind_INSTANCE_TAG_DETACHED,
					Package:  "pkg",
					Instance: digest,
					Tag:      "a:4",
					Who:      string(testUser),
					When:     google.NewTimestamp(testTime.Add(3*time.Second + 1)),
				},
				{
					Kind:     api.EventKind_INSTANCE_TAG_DETACHED,
					Package:  "pkg",
					Instance: digest,
					Tag:      "a:3",
					Who:      string(testUser),
					When:     google.NewTimestamp(testTime.Add(3 * time.Second)),
				},
				{
					Kind:     api.EventKind_INSTANCE_TAG_DETACHED,
					Package:  "pkg",
					Instance: digest,
					Tag:      "a:2",
					Who:      string(testUser),
					When:     google.NewTimestamp(testTime.Add(2*time.Second + 1)),
				},
				{
					Kind:     api.EventKind_INSTANCE_TAG_DETACHED,
					Package:  "pkg",
					Instance: digest,
					Tag:      "a:1",
					Who:      string(testUser),
					When:     google.NewTimestamp(testTime.Add(2 * time.Second)),
				},
				{
					Kind:     api.EventKind_INSTANCE_TAG_DETACHED,
					Package:  "pkg",
					Instance: digest,
					Tag:      "a:0",
					Who:      string(testUser),
					When:     google.NewTimestamp(testTime.Add(time.Second)),
				},
			})
		})

		Convey("AttachTags to not ready instance", func() {
			inst := putInst("pkg", digest, []string{"proc"})

			err := AttachTags(ctx, inst, tags("a:0"))
			So(grpcutil.Code(err), ShouldEqual, codes.FailedPrecondition)
			So(err, ShouldErrLike, "the instance is not ready yet")
		})

		Convey("Handles SHA1 collision", func() {
			inst := putInst("pkg", digest, nil)

			// We fake a collision here. Coming up with a real SHA1 collision to use
			// in this test is left as an exercise to the reader.
			So(datastore.Put(ctx, &Tag{
				ID:       TagID(common.MustParseInstanceTag("some:tag")),
				Instance: datastore.KeyForObj(ctx, inst),
				Tag:      "another:tag",
			}), ShouldBeNil)

			Convey("AttachTags", func() {
				err := AttachTags(ctx, inst, tags("some:tag"))
				So(grpcutil.Code(err), ShouldEqual, codes.Internal)
				So(err, ShouldErrLike, `tag "some:tag" collides with tag "another:tag", refusing to touch it`)
			})

			Convey("DetachTags", func() {
				err := DetachTags(ctx, inst, tags("some:tag"))
				So(grpcutil.Code(err), ShouldEqual, codes.Internal)
				So(err, ShouldErrLike, `tag "some:tag" collides with tag "another:tag", refusing to touch it`)
			})
		})

		Convey("ResolveTag works", func() {
			inst1 := putInst("pkg", strings.Repeat("1", 40), nil)
			inst2 := putInst("pkg", strings.Repeat("2", 40), nil)

			AttachTags(ctx, inst1, tags("ver:1", "ver:ambiguous"))
			AttachTags(ctx, inst2, tags("ver:2", "ver:ambiguous"))

			Convey("Happy path", func() {
				iid, err := ResolveTag(ctx, "pkg", common.MustParseInstanceTag("ver:1"))
				So(err, ShouldBeNil)
				So(iid, ShouldEqual, inst1.InstanceID)
			})

			Convey("No such tag", func() {
				_, err := ResolveTag(ctx, "pkg", common.MustParseInstanceTag("ver:zzz"))
				So(grpcutil.Code(err), ShouldEqual, codes.NotFound)
				So(err, ShouldErrLike, "no such tag")
			})

			Convey("Ambiguous tag", func() {
				_, err := ResolveTag(ctx, "pkg", common.MustParseInstanceTag("ver:ambiguous"))
				So(grpcutil.Code(err), ShouldEqual, codes.FailedPrecondition)
				So(err, ShouldErrLike, "ambiguity when resolving the tag")
			})
		})

		Convey("ListInstanceTags works", func() {
			asStr := func(tags []*Tag) []string {
				out := make([]string, len(tags))
				for i, t := range tags {
					out[i] = t.Tag
				}
				return out
			}

			inst := putInst("pkg", strings.Repeat("1", 40), nil)

			// Tags registered at the same time are sorted alphabetically.
			AttachTags(ctx, inst, tags("z:1", "b:2", "b:1"))
			t, err := ListInstanceTags(ctx, inst)
			So(err, ShouldBeNil)
			So(asStr(t), ShouldResemble, []string{"b:1", "b:2", "z:1"})

			tc.Add(time.Minute)

			// Tags are sorted by key first, and then by timestamp within the key.
			AttachTags(ctx, inst, tags("y:1", "a:1", "b:3"))
			t, err = ListInstanceTags(ctx, inst)
			So(err, ShouldBeNil)
			So(asStr(t), ShouldResemble, []string{
				"a:1",
				"b:3",
				"b:1",
				"b:2",
				"y:1",
				"z:1",
			})
		})
	})
}
