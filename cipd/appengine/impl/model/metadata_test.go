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

	api "go.chromium.org/luci/cipd/api/cipd/v1"
	"go.chromium.org/luci/cipd/appengine/impl/testutil"
	"go.chromium.org/luci/cipd/common"
)

func TestMetadata(t *testing.T) {
	t.Parallel()

	ftt.Run("With datastore", t, func(t *ftt.Test) {
		digest := strings.Repeat("a", 40)

		ctx, tc, as := testutil.TestingContext()

		fp := func(key, value string) string {
			return common.InstanceMetadataFingerprint(key, []byte(value))
		}

		putInst := func(pkg, iid string, pendingProcs []string) *Instance {
			inst := &Instance{
				InstanceID:        iid,
				Package:           PackageKey(ctx, pkg),
				ProcessorsPending: pendingProcs,
			}
			assert.Loosely(t, datastore.Put(ctx, &Package{Name: pkg}, inst), should.BeNil)
			return inst
		}

		// Fetches metadata entity from datastore.
		getMD := func(key, value string, inst *Instance) *InstanceMetadata {
			ent := &InstanceMetadata{
				Fingerprint: fp(key, value),
				Instance:    datastore.KeyForObj(ctx, inst),
			}
			if err := datastore.Get(ctx, ent); err != datastore.ErrNoSuchEntity {
				assert.Loosely(t, err, should.BeNil)
				return ent
			}
			return nil
		}

		// Constructs expected metadata entity.
		expMD := func(key, value string, inst *Instance, ct string, secs int, who identity.Identity) *InstanceMetadata {
			return &InstanceMetadata{
				Fingerprint: fp(key, value),
				Instance:    datastore.KeyForObj(ctx, inst),
				Key:         key,
				Value:       []byte(value),
				ContentType: ct,
				AttachedBy:  string(who),
				AttachedTs:  testutil.TestTime.Add(time.Duration(secs) * time.Second),
			}
		}

		t.Run("AttachMetadata happy paths", func(t *ftt.Test) {
			inst := putInst("pkg", digest, nil)

			// Attach one entry and verify it exist.
			assert.Loosely(t, AttachMetadata(ctx, inst, []*api.InstanceMetadata{
				{
					Key:   "key",
					Value: []byte("some value"),
				},
			}), should.BeNil)
			assert.Loosely(t,
				getMD("key", "some value", inst), should.Resemble(
					expMD("key", "some value", inst, "text/plain", 0, testutil.TestUser),
				))

			// Attach few more at once.
			tc.Add(time.Second)
			assert.Loosely(t, AttachMetadata(ctx, inst, []*api.InstanceMetadata{
				{
					Key:         "key",
					Value:       []byte("some value 1"),
					ContentType: "text/plain",
				},
				{
					Key:         "another",
					Value:       []byte("some value 2"),
					ContentType: "application/octet-stream",
				},
			}), should.BeNil)
			assert.Loosely(t,
				getMD("key", "some value 1", inst), should.Resemble(
					expMD("key", "some value 1", inst, "text/plain", 1, testutil.TestUser),
				))
			assert.Loosely(t,
				getMD("another", "some value 2", inst), should.Resemble(
					expMD("another", "some value 2", inst, "application/octet-stream", 1, testutil.TestUser),
				))

			// Try to reattach an existing one (notice the change in the email),
			// should be ignored.
			assert.Loosely(t, AttachMetadata(as("zzz@example.com"), inst, []*api.InstanceMetadata{
				{
					Key:         "key",
					Value:       []byte("some value"),
					ContentType: "ignored when dedupping",
				},
			}), should.BeNil)
			assert.Loosely(t,
				getMD("key", "some value", inst), should.Resemble(
					expMD("key", "some value", inst, "text/plain", 0, testutil.TestUser),
				))

			// Try to reattach a bunch of existing ones at once.
			assert.Loosely(t, AttachMetadata(as("zzz@example.com"), inst, []*api.InstanceMetadata{
				{
					Key:         "key",
					Value:       []byte("some value 1"),
					ContentType: "ignored when dedupping",
				},
				{
					Key:         "another",
					Value:       []byte("some value 2"),
					ContentType: "ignored when dedupping",
				},
			}), should.BeNil)
			assert.Loosely(t,
				getMD("key", "some value 1", inst), should.Resemble(
					expMD("key", "some value 1", inst, "text/plain", 1, testutil.TestUser),
				))
			assert.Loosely(t,
				getMD("another", "some value 2", inst), should.Resemble(
					expMD("another", "some value 2", inst, "application/octet-stream", 1, testutil.TestUser),
				))

			// Mixed group with new and existing entries.
			tc.Add(time.Second)
			assert.Loosely(t, AttachMetadata(as("zzz@example.com"), inst, []*api.InstanceMetadata{
				{
					Key:         "key",
					Value:       []byte("some value 1"),
					ContentType: "ignored when dedupping",
				},
				{
					Key:         "new-key",
					Value:       []byte("value 1"),
					ContentType: "text/plain",
				},
				{
					Key:         "another",
					Value:       []byte("some value 2"),
					ContentType: "ignored when dedupping",
				},
				{
					Key:         "new-key",
					Value:       []byte("value 2"),
					ContentType: "text/plain",
				},
			}), should.BeNil)
			assert.Loosely(t,
				getMD("key", "some value 1", inst), should.Resemble(
					expMD("key", "some value 1", inst, "text/plain", 1, testutil.TestUser),
				))
			assert.Loosely(t,
				getMD("new-key", "value 1", inst), should.Resemble(
					expMD("new-key", "value 1", inst, "text/plain", 2, "user:zzz@example.com"),
				))
			assert.Loosely(t,
				getMD("another", "some value 2", inst), should.Resemble(
					expMD("another", "some value 2", inst, "application/octet-stream", 1, testutil.TestUser),
				))
			assert.Loosely(t,
				getMD("new-key", "value 2", inst), should.Resemble(
					expMD("new-key", "value 2", inst, "text/plain", 2, "user:zzz@example.com"),
				))

			// A duplicate in a batch. The first one wins.
			assert.Loosely(t, AttachMetadata(ctx, inst, []*api.InstanceMetadata{
				{
					Key:         "dup-key",
					Value:       []byte("dup-value"),
					ContentType: "text/1",
				},
				{
					Key:         "dup-key",
					Value:       []byte("dup-value"),
					ContentType: "text/2",
				},
			}), should.BeNil)
			assert.Loosely(t,
				getMD("dup-key", "dup-value", inst), should.Resemble(
					expMD("dup-key", "dup-value", inst, "text/1", 2, testutil.TestUser),
				))

			// All events have been collected.
			assert.Loosely(t, GetEvents(ctx), should.Resemble([]*api.Event{
				{
					Kind:          api.EventKind_INSTANCE_METADATA_ATTACHED,
					Package:       "pkg",
					Instance:      digest,
					Who:           "user:zzz@example.com",
					When:          timestamppb.New(testutil.TestTime.Add(2*time.Second + 1)),
					MdKey:         "new-key",
					MdValue:       "value 2",
					MdContentType: "text/plain",
					MdFingerprint: fp("new-key", "value 2"),
				},
				{
					Kind:          api.EventKind_INSTANCE_METADATA_ATTACHED,
					Package:       "pkg",
					Instance:      digest,
					Who:           "user:zzz@example.com",
					When:          timestamppb.New(testutil.TestTime.Add(2 * time.Second)),
					MdKey:         "new-key",
					MdValue:       "value 1",
					MdContentType: "text/plain",
					MdFingerprint: fp("new-key", "value 1"),
				},
				{
					Kind:          api.EventKind_INSTANCE_METADATA_ATTACHED,
					Package:       "pkg",
					Instance:      digest,
					Who:           string(testutil.TestUser),
					When:          timestamppb.New(testutil.TestTime.Add(2 * time.Second)),
					MdKey:         "dup-key",
					MdValue:       "dup-value",
					MdContentType: "text/1",
					MdFingerprint: fp("dup-key", "dup-value"),
				},
				{
					Kind:          api.EventKind_INSTANCE_METADATA_ATTACHED,
					Package:       "pkg",
					Instance:      digest,
					Who:           string(testutil.TestUser),
					When:          timestamppb.New(testutil.TestTime.Add(1*time.Second + 1)),
					MdKey:         "another",
					MdContentType: "application/octet-stream",
					MdFingerprint: fp("another", "some value 2"),
				},
				{
					Kind:          api.EventKind_INSTANCE_METADATA_ATTACHED,
					Package:       "pkg",
					Instance:      digest,
					Who:           string(testutil.TestUser),
					When:          timestamppb.New(testutil.TestTime.Add(1 * time.Second)),
					MdKey:         "key",
					MdValue:       "some value 1",
					MdContentType: "text/plain",
					MdFingerprint: fp("key", "some value 1"),
				},
				{
					Kind:          api.EventKind_INSTANCE_METADATA_ATTACHED,
					Package:       "pkg",
					Instance:      digest,
					Who:           string(testutil.TestUser),
					When:          timestamppb.New(testutil.TestTime),
					MdKey:         "key",
					MdValue:       "some value",
					MdContentType: "text/plain",
					MdFingerprint: fp("key", "some value"),
				},
			}))
		})

		t.Run("AttachMetadata to not ready instance", func(t *ftt.Test) {
			inst := putInst("pkg", digest, []string{"proc"})

			err := AttachMetadata(ctx, inst, []*api.InstanceMetadata{
				{
					Key:   "key",
					Value: []byte("some value"),
				},
			})
			assert.Loosely(t, grpcutil.Code(err), should.Equal(codes.FailedPrecondition))
			assert.Loosely(t, err, should.ErrLike("the instance is not ready yet"))
		})

		t.Run("DetachMetadata happy paths", func(t *ftt.Test) {
			inst := putInst("pkg", digest, nil)

			// Attach a bunch of metadata first, so we have something to detach.
			assert.Loosely(t, AttachMetadata(ctx, inst, []*api.InstanceMetadata{
				{Key: "a", Value: []byte("0"), ContentType: "text/0"},
				{Key: "a", Value: []byte("1"), ContentType: "text/1"},
				{Key: "a", Value: []byte("2"), ContentType: "text/2"},
				{Key: "a", Value: []byte("3"), ContentType: "text/3"},
				{Key: "a", Value: []byte("4"), ContentType: "text/4"},
				{Key: "a", Value: []byte("5"), ContentType: "text/5"},
			}), should.BeNil)

			// Detach one existing using a key-value pair.
			tc.Add(time.Second)
			assert.Loosely(t, getMD("a", "0", inst), should.NotBeNil)
			assert.Loosely(t, DetachMetadata(ctx, inst, []*api.InstanceMetadata{
				{Key: "a", Value: []byte("0")},
			}), should.BeNil)
			assert.Loosely(t, getMD("a", "0", inst), should.BeNil)

			// Detach one existing using a fingerprint pair.
			tc.Add(time.Second)
			assert.Loosely(t, getMD("a", "1", inst), should.NotBeNil)
			assert.Loosely(t, DetachMetadata(ctx, inst, []*api.InstanceMetadata{
				{Fingerprint: fp("a", "1")},
			}), should.BeNil)
			assert.Loosely(t, getMD("a", "1", inst), should.BeNil)

			// Detach one missing.
			assert.Loosely(t, DetachMetadata(ctx, inst, []*api.InstanceMetadata{
				{Key: "a", Value: []byte("z0")},
			}), should.BeNil)

			// Detach a bunch of existing, including dups.
			tc.Add(time.Second)
			assert.Loosely(t, getMD("a", "2", inst), should.NotBeNil)
			assert.Loosely(t, getMD("a", "3", inst), should.NotBeNil)
			assert.Loosely(t, DetachMetadata(ctx, inst, []*api.InstanceMetadata{
				{Key: "a", Value: []byte("2")},
				{Key: "a", Value: []byte("3")},
				{Key: "a", Value: []byte("2")},
				{Fingerprint: fp("a", "3")},
			}), should.BeNil)
			assert.Loosely(t, getMD("a", "2", inst), should.BeNil)
			assert.Loosely(t, getMD("a", "3", inst), should.BeNil)

			// Detach a bunch of missing.
			assert.Loosely(t, DetachMetadata(ctx, inst, []*api.InstanceMetadata{
				{Key: "a", Value: []byte("z1")},
				{Key: "a", Value: []byte("z2")},
			}), should.BeNil)

			// Detach a mix of existing and missing.
			tc.Add(time.Second)
			assert.Loosely(t, getMD("a", "4", inst), should.NotBeNil)
			assert.Loosely(t, getMD("a", "5", inst), should.NotBeNil)
			assert.Loosely(t, DetachMetadata(ctx, inst, []*api.InstanceMetadata{
				{Key: "a", Value: []byte("z3")},
				{Key: "a", Value: []byte("4")},
				{Key: "a", Value: []byte("z4")},
				{Key: "a", Value: []byte("5")},
			}), should.BeNil)
			assert.Loosely(t, getMD("a", "4", inst), should.BeNil)
			assert.Loosely(t, getMD("a", "5", inst), should.BeNil)

			// All 'detach' events have been collected (skip checking 'attach' ones).
			assert.Loosely(t, GetEvents(ctx)[:6], should.Resemble([]*api.Event{
				{
					Kind:          api.EventKind_INSTANCE_METADATA_DETACHED,
					Package:       "pkg",
					Instance:      digest,
					Who:           string(testutil.TestUser),
					When:          timestamppb.New(testutil.TestTime.Add(4*time.Second + 1)),
					MdKey:         "a",
					MdValue:       "5",
					MdContentType: "text/5",
					MdFingerprint: fp("a", "5"),
				},
				{
					Kind:          api.EventKind_INSTANCE_METADATA_DETACHED,
					Package:       "pkg",
					Instance:      digest,
					Who:           string(testutil.TestUser),
					When:          timestamppb.New(testutil.TestTime.Add(4 * time.Second)),
					MdKey:         "a",
					MdValue:       "4",
					MdContentType: "text/4",
					MdFingerprint: fp("a", "4"),
				},
				{
					Kind:          api.EventKind_INSTANCE_METADATA_DETACHED,
					Package:       "pkg",
					Instance:      digest,
					Who:           string(testutil.TestUser),
					When:          timestamppb.New(testutil.TestTime.Add(3*time.Second + 1)),
					MdKey:         "a",
					MdValue:       "3",
					MdContentType: "text/3",
					MdFingerprint: fp("a", "3"),
				},
				{
					Kind:          api.EventKind_INSTANCE_METADATA_DETACHED,
					Package:       "pkg",
					Instance:      digest,
					Who:           string(testutil.TestUser),
					When:          timestamppb.New(testutil.TestTime.Add(3 * time.Second)),
					MdKey:         "a",
					MdValue:       "2",
					MdContentType: "text/2",
					MdFingerprint: fp("a", "2"),
				},
				{
					Kind:          api.EventKind_INSTANCE_METADATA_DETACHED,
					Package:       "pkg",
					Instance:      digest,
					Who:           string(testutil.TestUser),
					When:          timestamppb.New(testutil.TestTime.Add(2 * time.Second)),
					MdKey:         "a",
					MdValue:       "1",
					MdContentType: "text/1",
					MdFingerprint: fp("a", "1"),
				},
				{
					Kind:          api.EventKind_INSTANCE_METADATA_DETACHED,
					Package:       "pkg",
					Instance:      digest,
					Who:           string(testutil.TestUser),
					When:          timestamppb.New(testutil.TestTime.Add(1 * time.Second)),
					MdKey:         "a",
					MdValue:       "0",
					MdContentType: "text/0",
					MdFingerprint: fp("a", "0"),
				},
			}))
		})

		t.Run("Listing works", func(t *ftt.Test) {
			inst := putInst("pkg", digest, nil)

			assert.Loosely(t, AttachMetadata(ctx, inst, []*api.InstanceMetadata{
				{Key: "a", Value: []byte("0")},
				{Key: "b", Value: []byte("0")},
				{Key: "c", Value: []byte("0")},
			}), should.BeNil)

			tc.Add(time.Second)
			assert.Loosely(t, AttachMetadata(ctx, inst, []*api.InstanceMetadata{
				{Key: "a", Value: []byte("1")},
				{Key: "b", Value: []byte("1")},
				{Key: "c", Value: []byte("1")},
			}), should.BeNil)

			tc.Add(time.Second)
			assert.Loosely(t, AttachMetadata(ctx, inst, []*api.InstanceMetadata{
				{Key: "a", Value: []byte("2")},
				{Key: "b", Value: []byte("2")},
				{Key: "c", Value: []byte("2")},
			}), should.BeNil)

			pairs := func(md []*InstanceMetadata) []string {
				var out []string
				for _, m := range md {
					out = append(out, m.Key+":"+string(m.Value))
				}
				return out
			}

			t.Run("ListMetadata", func(t *ftt.Test) {
				t.Run("Some", func(t *ftt.Test) {
					md, err := ListMetadata(ctx, inst)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, pairs(md), should.Resemble([]string{
						"a:2", "b:2", "c:2", "a:1", "b:1", "c:1", "a:0", "b:0", "c:0",
					}))
				})
				t.Run("None", func(t *ftt.Test) {
					inst := putInst("another/pkg", digest, nil)
					md, err := ListMetadata(ctx, inst)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, md, should.HaveLength(0))
				})
			})

			t.Run("ListMetadataWithKeys", func(t *ftt.Test) {
				t.Run("Some", func(t *ftt.Test) {
					md, err := ListMetadataWithKeys(ctx, inst, []string{"a", "c", "missing"})
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, pairs(md), should.Resemble([]string{
						"a:2", "c:2", "a:1", "c:1", "a:0", "c:0",
					}))
				})
				t.Run("None", func(t *ftt.Test) {
					md, err := ListMetadataWithKeys(ctx, inst, []string{"missing"})
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, md, should.HaveLength(0))
				})
			})
		})
	})
}

func TestGuessPlainText(t *testing.T) {
	t.Parallel()

	ftt.Run("guessPlainText works", t, func(t *ftt.Test) {
		assert.Loosely(t, guessPlainText([]byte("")), should.BeTrue)
		assert.Loosely(t, guessPlainText([]byte("abc")), should.BeTrue)
		assert.Loosely(t, guessPlainText([]byte(" ~\t\r\n")), should.BeTrue)
		assert.Loosely(t, guessPlainText([]byte{0x19}), should.BeFalse)
		assert.Loosely(t, guessPlainText([]byte{0x7F}), should.BeFalse)
	})
}
