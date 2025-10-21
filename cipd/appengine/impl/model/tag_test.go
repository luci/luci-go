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
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcutil"

	repopb "go.chromium.org/luci/cipd/api/cipd/v1/repopb"
	"go.chromium.org/luci/cipd/appengine/impl/testutil"
	"go.chromium.org/luci/cipd/common"
)

func TestTags(t *testing.T) {
	t.Parallel()

	ftt.Run("With datastore", t, func(t *ftt.Test) {
		digest := strings.Repeat("a", 40)

		ctx, tc, as := testutil.TestingContext()

		putInst := func(pkg, iid string, pendingProcs []string) *Instance {
			inst := &Instance{
				InstanceID:        iid,
				Package:           PackageKey(ctx, pkg),
				ProcessorsPending: pendingProcs,
			}
			assert.Loosely(t, datastore.Put(ctx, &Package{Name: pkg}, inst), should.BeNil)
			return inst
		}

		tags := func(tag ...string) []*repopb.Tag {
			out := make([]*repopb.Tag, len(tag))
			for i, s := range tag {
				out[i] = common.MustParseInstanceTag(s)
			}
			return out
		}

		getTag := func(inst *Instance, tag string) *Tag {
			mTag := &Tag{
				ID:       TagID(common.MustParseInstanceTag(tag)),
				Instance: datastore.KeyForObj(ctx, inst),
			}
			err := datastore.Get(ctx, mTag)
			if err == datastore.ErrNoSuchEntity {
				return nil
			}
			assert.Loosely(t, err, should.BeNil)
			return mTag
		}

		expectedTag := func(inst *Instance, tag string, secs int, who identity.Identity) *Tag {
			return &Tag{
				ID:           TagID(common.MustParseInstanceTag(tag)),
				Instance:     datastore.KeyForObj(ctx, inst),
				Tag:          tag,
				RegisteredBy: string(who),
				RegisteredTs: testutil.TestTime.Add(time.Duration(secs) * time.Second),
			}
		}

		t.Run("Proto() works", func(t *ftt.Test) {
			inst := putInst("pkg", digest, nil)
			mTag := expectedTag(inst, "k:v", 0, testutil.TestUser)
			assert.Loosely(t, mTag.Proto(), should.Resemble(&repopb.Tag{
				Key:        "k",
				Value:      "v",
				AttachedBy: string(testutil.TestUser),
				AttachedTs: timestamppb.New(testutil.TestTime),
			}))
		})

		t.Run("AttachTags happy paths", func(t *ftt.Test) {
			inst := putInst("pkg", digest, nil)

			// Attach one tag and verify it exist.
			assert.Loosely(t, AttachTags(ctx, inst, tags("a:0")), should.BeNil)
			assert.Loosely(t, getTag(inst, "a:0"), should.Resemble(expectedTag(inst, "a:0", 0, testutil.TestUser)))

			// Attach few more at once.
			tc.Add(time.Second)
			assert.Loosely(t, AttachTags(ctx, inst, tags("a:1", "a:2")), should.BeNil)
			assert.Loosely(t, getTag(inst, "a:1"), should.Resemble(expectedTag(inst, "a:1", 1, testutil.TestUser)))
			assert.Loosely(t, getTag(inst, "a:2"), should.Resemble(expectedTag(inst, "a:2", 1, testutil.TestUser)))

			// Try to reattach an existing one (notice the change in the email),
			// should be ignored.
			assert.Loosely(t, AttachTags(as("def@example.com"), inst, tags("a:0")), should.BeNil)
			assert.Loosely(t, getTag(inst, "a:0"), should.Resemble(expectedTag(inst, "a:0", 0, testutil.TestUser)))

			// Try to reattach a bunch of existing ones at once.
			assert.Loosely(t, AttachTags(as("def@example.com"), inst, tags("a:1", "a:2")), should.BeNil)
			assert.Loosely(t, getTag(inst, "a:1"), should.Resemble(expectedTag(inst, "a:1", 1, testutil.TestUser)))
			assert.Loosely(t, getTag(inst, "a:2"), should.Resemble(expectedTag(inst, "a:2", 1, testutil.TestUser)))

			// Mixed group with new and existing tags.
			tc.Add(time.Second)
			assert.Loosely(t, AttachTags(as("def@example.com"), inst, tags("a:3", "a:0", "a:4", "a:1")), should.BeNil)
			assert.Loosely(t, getTag(inst, "a:3"), should.Resemble(expectedTag(inst, "a:3", 2, "user:def@example.com")))
			assert.Loosely(t, getTag(inst, "a:0"), should.Resemble(expectedTag(inst, "a:0", 0, testutil.TestUser)))
			assert.Loosely(t, getTag(inst, "a:4"), should.Resemble(expectedTag(inst, "a:4", 2, "user:def@example.com")))
			assert.Loosely(t, getTag(inst, "a:1"), should.Resemble(expectedTag(inst, "a:1", 1, testutil.TestUser)))

			// All events have been collected.
			assert.Loosely(t, GetEvents(ctx), should.Resemble([]*repopb.Event{
				{
					Kind:     repopb.EventKind_INSTANCE_TAG_ATTACHED,
					Package:  "pkg",
					Instance: digest,
					Tag:      "a:4",
					Who:      "user:def@example.com",
					When:     timestamppb.New(testutil.TestTime.Add(2*time.Second + 1)),
				},
				{
					Kind:     repopb.EventKind_INSTANCE_TAG_ATTACHED,
					Package:  "pkg",
					Instance: digest,
					Tag:      "a:3",
					Who:      "user:def@example.com",
					When:     timestamppb.New(testutil.TestTime.Add(2 * time.Second)),
				},
				{
					Kind:     repopb.EventKind_INSTANCE_TAG_ATTACHED,
					Package:  "pkg",
					Instance: digest,
					Tag:      "a:2",
					Who:      string(testutil.TestUser),
					When:     timestamppb.New(testutil.TestTime.Add(1*time.Second + 1)),
				},
				{
					Kind:     repopb.EventKind_INSTANCE_TAG_ATTACHED,
					Package:  "pkg",
					Instance: digest,
					Tag:      "a:1",
					Who:      string(testutil.TestUser),
					When:     timestamppb.New(testutil.TestTime.Add(1 * time.Second)),
				},
				{
					Kind:     repopb.EventKind_INSTANCE_TAG_ATTACHED,
					Package:  "pkg",
					Instance: digest,
					Tag:      "a:0",
					Who:      string(testutil.TestUser),
					When:     timestamppb.New(testutil.TestTime),
				},
			}))
		})

		t.Run("DetachTags happy paths", func(t *ftt.Test) {
			inst := putInst("pkg", digest, nil)

			// Attach a bunch of tags first, so we have something to detach.
			assert.Loosely(t, AttachTags(ctx, inst, tags("a:0", "a:1", "a:2", "a:3", "a:4")), should.BeNil)

			// Detaching one existing.
			tc.Add(time.Second)
			assert.Loosely(t, getTag(inst, "a:0"), should.NotBeNil)
			assert.Loosely(t, DetachTags(ctx, inst, tags("a:0")), should.BeNil)
			assert.Loosely(t, getTag(inst, "a:0"), should.BeNil)

			// Detaching one missing.
			assert.Loosely(t, DetachTags(ctx, inst, tags("a:z0")), should.BeNil)

			// Detaching a bunch of existing.
			tc.Add(time.Second)
			assert.Loosely(t, getTag(inst, "a:1"), should.NotBeNil)
			assert.Loosely(t, getTag(inst, "a:2"), should.NotBeNil)
			assert.Loosely(t, DetachTags(ctx, inst, tags("a:1", "a:2")), should.BeNil)
			assert.Loosely(t, getTag(inst, "a:1"), should.BeNil)
			assert.Loosely(t, getTag(inst, "a:2"), should.BeNil)

			// Detaching a bunch of missing.
			assert.Loosely(t, DetachTags(ctx, inst, tags("a:z1", "a:z2")), should.BeNil)

			// Detaching a mix of existing and missing.
			tc.Add(time.Second)
			assert.Loosely(t, getTag(inst, "a:3"), should.NotBeNil)
			assert.Loosely(t, getTag(inst, "a:4"), should.NotBeNil)
			assert.Loosely(t, DetachTags(ctx, inst, tags("a:z3", "a:3", "a:z4", "a:4")), should.BeNil)
			assert.Loosely(t, getTag(inst, "a:3"), should.BeNil)
			assert.Loosely(t, getTag(inst, "a:4"), should.BeNil)

			// All 'detach' events have been collected (skip checking 'attach' ones).
			assert.Loosely(t, GetEvents(ctx)[:5], should.Resemble([]*repopb.Event{
				{
					Kind:     repopb.EventKind_INSTANCE_TAG_DETACHED,
					Package:  "pkg",
					Instance: digest,
					Tag:      "a:4",
					Who:      string(testutil.TestUser),
					When:     timestamppb.New(testutil.TestTime.Add(3*time.Second + 1)),
				},
				{
					Kind:     repopb.EventKind_INSTANCE_TAG_DETACHED,
					Package:  "pkg",
					Instance: digest,
					Tag:      "a:3",
					Who:      string(testutil.TestUser),
					When:     timestamppb.New(testutil.TestTime.Add(3 * time.Second)),
				},
				{
					Kind:     repopb.EventKind_INSTANCE_TAG_DETACHED,
					Package:  "pkg",
					Instance: digest,
					Tag:      "a:2",
					Who:      string(testutil.TestUser),
					When:     timestamppb.New(testutil.TestTime.Add(2*time.Second + 1)),
				},
				{
					Kind:     repopb.EventKind_INSTANCE_TAG_DETACHED,
					Package:  "pkg",
					Instance: digest,
					Tag:      "a:1",
					Who:      string(testutil.TestUser),
					When:     timestamppb.New(testutil.TestTime.Add(2 * time.Second)),
				},
				{
					Kind:     repopb.EventKind_INSTANCE_TAG_DETACHED,
					Package:  "pkg",
					Instance: digest,
					Tag:      "a:0",
					Who:      string(testutil.TestUser),
					When:     timestamppb.New(testutil.TestTime.Add(time.Second)),
				},
			}))
		})

		t.Run("AttachTags to not ready instance", func(t *ftt.Test) {
			inst := putInst("pkg", digest, []string{"proc"})

			err := AttachTags(ctx, inst, tags("a:0"))
			assert.Loosely(t, grpcutil.Code(err), should.Equal(codes.FailedPrecondition))
			assert.Loosely(t, err, should.ErrLike("the instance is not ready yet"))
		})

		t.Run("Handles SHA1 collision", func(t *ftt.Test) {
			inst := putInst("pkg", digest, nil)

			// We fake a collision here. Coming up with a real SHA1 collision to use
			// in this test is left as an exercise to the reader.
			assert.Loosely(t, datastore.Put(ctx, &Tag{
				ID:       TagID(common.MustParseInstanceTag("some:tag")),
				Instance: datastore.KeyForObj(ctx, inst),
				Tag:      "another:tag",
			}), should.BeNil)

			t.Run("AttachTags", func(t *ftt.Test) {
				err := AttachTags(ctx, inst, tags("some:tag"))
				assert.Loosely(t, grpcutil.Code(err), should.Equal(codes.Internal))
				assert.Loosely(t, err, should.ErrLike(`tag "some:tag" collides with tag "another:tag", refusing to touch it`))
			})

			t.Run("DetachTags", func(t *ftt.Test) {
				err := DetachTags(ctx, inst, tags("some:tag"))
				assert.Loosely(t, grpcutil.Code(err), should.Equal(codes.Internal))
				assert.Loosely(t, err, should.ErrLike(`tag "some:tag" collides with tag "another:tag", refusing to touch it`))
			})
		})

		t.Run("ResolveTag works", func(t *ftt.Test) {
			inst1 := putInst("pkg", strings.Repeat("1", 40), nil)
			inst2 := putInst("pkg", strings.Repeat("2", 40), nil)

			AttachTags(ctx, inst1, tags("ver:1", "ver:ambiguous"))
			AttachTags(ctx, inst2, tags("ver:2", "ver:ambiguous"))

			t.Run("Happy path", func(t *ftt.Test) {
				iid, err := ResolveTag(ctx, "pkg", common.MustParseInstanceTag("ver:1"))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, iid, should.Equal(inst1.InstanceID))
			})

			t.Run("No such tag", func(t *ftt.Test) {
				_, err := ResolveTag(ctx, "pkg", common.MustParseInstanceTag("ver:zzz"))
				assert.Loosely(t, grpcutil.Code(err), should.Equal(codes.NotFound))
				assert.Loosely(t, err, should.ErrLike("no such tag"))
			})

			t.Run("Ambiguous tag", func(t *ftt.Test) {
				_, err := ResolveTag(ctx, "pkg", common.MustParseInstanceTag("ver:ambiguous"))
				assert.Loosely(t, grpcutil.Code(err), should.Equal(codes.FailedPrecondition))
				assert.Loosely(t, err, should.ErrLike("ambiguity when resolving the tag"))
			})
		})

		t.Run("ListInstanceTags works", func(t *ftt.Test) {
			asStr := func(tags []*Tag) []string {
				out := make([]string, len(tags))
				for i, mTag := range tags {
					out[i] = mTag.Tag
				}
				return out
			}

			inst := putInst("pkg", strings.Repeat("1", 40), nil)

			// Tags registered at the same time are sorted alphabetically.
			AttachTags(ctx, inst, tags("z:1", "b:2", "b:1"))
			mTag, err := ListInstanceTags(ctx, inst)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, asStr(mTag), should.Resemble([]string{"b:1", "b:2", "z:1"}))

			tc.Add(time.Minute)

			// Tags are sorted by key first, and then by timestamp within the key.
			AttachTags(ctx, inst, tags("y:1", "a:1", "b:3"))
			mTag, err = ListInstanceTags(ctx, inst)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, asStr(mTag), should.Resemble([]string{
				"a:1",
				"b:3",
				"b:1",
				"b:2",
				"y:1",
				"z:1",
			}))
		})
	})
}
