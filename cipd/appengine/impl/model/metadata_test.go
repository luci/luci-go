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
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcutil"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
	"go.chromium.org/luci/cipd/appengine/impl/testutil"
	"go.chromium.org/luci/cipd/common"

	. "github.com/smartystreets/goconvey/convey"

	. "go.chromium.org/luci/common/testing/assertions"
)

func TestMetadata(t *testing.T) {
	t.Parallel()

	Convey("With datastore", t, func() {
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
			So(datastore.Put(ctx, &Package{Name: pkg}, inst), ShouldBeNil)
			return inst
		}

		// Fetches metadata entity from datastore.
		getMD := func(key, value string, inst *Instance) *InstanceMetadata {
			ent := &InstanceMetadata{
				Fingerprint: fp(key, value),
				Instance:    datastore.KeyForObj(ctx, inst),
			}
			if err := datastore.Get(ctx, ent); err != datastore.ErrNoSuchEntity {
				So(err, ShouldBeNil)
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

		Convey("AttachMetadata happy paths", func() {
			inst := putInst("pkg", digest, nil)

			// Attach one entry and verify it exist.
			So(AttachMetadata(ctx, inst, []*api.InstanceMetadata{
				{
					Key:   "key",
					Value: []byte("some value"),
				},
			}), ShouldBeNil)
			So(
				getMD("key", "some value", inst), ShouldResemble,
				expMD("key", "some value", inst, "text/plain", 0, testutil.TestUser),
			)

			// Attach few more at once.
			tc.Add(time.Second)
			So(AttachMetadata(ctx, inst, []*api.InstanceMetadata{
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
			}), ShouldBeNil)
			So(
				getMD("key", "some value 1", inst), ShouldResemble,
				expMD("key", "some value 1", inst, "text/plain", 1, testutil.TestUser),
			)
			So(
				getMD("another", "some value 2", inst), ShouldResemble,
				expMD("another", "some value 2", inst, "application/octet-stream", 1, testutil.TestUser),
			)

			// Try to reattach an existing one (notice the change in the email),
			// should be ignored.
			So(AttachMetadata(as("zzz@example.com"), inst, []*api.InstanceMetadata{
				{
					Key:         "key",
					Value:       []byte("some value"),
					ContentType: "ignored when dedupping",
				},
			}), ShouldBeNil)
			So(
				getMD("key", "some value", inst), ShouldResemble,
				expMD("key", "some value", inst, "text/plain", 0, testutil.TestUser),
			)

			// Try to reattach a bunch of existing ones at once.
			So(AttachMetadata(as("zzz@example.com"), inst, []*api.InstanceMetadata{
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
			}), ShouldBeNil)
			So(
				getMD("key", "some value 1", inst), ShouldResemble,
				expMD("key", "some value 1", inst, "text/plain", 1, testutil.TestUser),
			)
			So(
				getMD("another", "some value 2", inst), ShouldResemble,
				expMD("another", "some value 2", inst, "application/octet-stream", 1, testutil.TestUser),
			)

			// Mixed group with new and existing entries.
			tc.Add(time.Second)
			So(AttachMetadata(as("zzz@example.com"), inst, []*api.InstanceMetadata{
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
			}), ShouldBeNil)
			So(
				getMD("key", "some value 1", inst), ShouldResemble,
				expMD("key", "some value 1", inst, "text/plain", 1, testutil.TestUser),
			)
			So(
				getMD("new-key", "value 1", inst), ShouldResemble,
				expMD("new-key", "value 1", inst, "text/plain", 2, "user:zzz@example.com"),
			)
			So(
				getMD("another", "some value 2", inst), ShouldResemble,
				expMD("another", "some value 2", inst, "application/octet-stream", 1, testutil.TestUser),
			)
			So(
				getMD("new-key", "value 2", inst), ShouldResemble,
				expMD("new-key", "value 2", inst, "text/plain", 2, "user:zzz@example.com"),
			)

			// A duplicate in a batch. The first one wins.
			So(AttachMetadata(ctx, inst, []*api.InstanceMetadata{
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
			}), ShouldBeNil)
			So(
				getMD("dup-key", "dup-value", inst), ShouldResemble,
				expMD("dup-key", "dup-value", inst, "text/1", 2, testutil.TestUser),
			)

			// All events have been collected.
			So(GetEvents(ctx), ShouldResembleProto, []*api.Event{
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
			})
		})

		Convey("AttachMetadata to not ready instance", func() {
			inst := putInst("pkg", digest, []string{"proc"})

			err := AttachMetadata(ctx, inst, []*api.InstanceMetadata{
				{
					Key:   "key",
					Value: []byte("some value"),
				},
			})
			So(grpcutil.Code(err), ShouldEqual, codes.FailedPrecondition)
			So(err, ShouldErrLike, "the instance is not ready yet")
		})

		Convey("DetachMetadata happy paths", func() {
			inst := putInst("pkg", digest, nil)

			// Attach a bunch of metadata first, so we have something to detach.
			So(AttachMetadata(ctx, inst, []*api.InstanceMetadata{
				{Key: "a", Value: []byte("0"), ContentType: "text/0"},
				{Key: "a", Value: []byte("1"), ContentType: "text/1"},
				{Key: "a", Value: []byte("2"), ContentType: "text/2"},
				{Key: "a", Value: []byte("3"), ContentType: "text/3"},
				{Key: "a", Value: []byte("4"), ContentType: "text/4"},
				{Key: "a", Value: []byte("5"), ContentType: "text/5"},
			}), ShouldBeNil)

			// Detach one existing using a key-value pair.
			tc.Add(time.Second)
			So(getMD("a", "0", inst), ShouldNotBeNil)
			So(DetachMetadata(ctx, inst, []*api.InstanceMetadata{
				{Key: "a", Value: []byte("0")},
			}), ShouldBeNil)
			So(getMD("a", "0", inst), ShouldBeNil)

			// Detach one existing using a fingerprint pair.
			tc.Add(time.Second)
			So(getMD("a", "1", inst), ShouldNotBeNil)
			So(DetachMetadata(ctx, inst, []*api.InstanceMetadata{
				{Fingerprint: fp("a", "1")},
			}), ShouldBeNil)
			So(getMD("a", "1", inst), ShouldBeNil)

			// Detach one missing.
			So(DetachMetadata(ctx, inst, []*api.InstanceMetadata{
				{Key: "a", Value: []byte("z0")},
			}), ShouldBeNil)

			// Detach a bunch of existing, including dups.
			tc.Add(time.Second)
			So(getMD("a", "2", inst), ShouldNotBeNil)
			So(getMD("a", "3", inst), ShouldNotBeNil)
			So(DetachMetadata(ctx, inst, []*api.InstanceMetadata{
				{Key: "a", Value: []byte("2")},
				{Key: "a", Value: []byte("3")},
				{Key: "a", Value: []byte("2")},
				{Fingerprint: fp("a", "3")},
			}), ShouldBeNil)
			So(getMD("a", "2", inst), ShouldBeNil)
			So(getMD("a", "3", inst), ShouldBeNil)

			// Detach a bunch of missing.
			So(DetachMetadata(ctx, inst, []*api.InstanceMetadata{
				{Key: "a", Value: []byte("z1")},
				{Key: "a", Value: []byte("z2")},
			}), ShouldBeNil)

			// Detach a mix of existing and missing.
			tc.Add(time.Second)
			So(getMD("a", "4", inst), ShouldNotBeNil)
			So(getMD("a", "5", inst), ShouldNotBeNil)
			So(DetachMetadata(ctx, inst, []*api.InstanceMetadata{
				{Key: "a", Value: []byte("z3")},
				{Key: "a", Value: []byte("4")},
				{Key: "a", Value: []byte("z4")},
				{Key: "a", Value: []byte("5")},
			}), ShouldBeNil)
			So(getMD("a", "4", inst), ShouldBeNil)
			So(getMD("a", "5", inst), ShouldBeNil)

			// All 'detach' events have been collected (skip checking 'attach' ones).
			So(GetEvents(ctx)[:6], ShouldResembleProto, []*api.Event{
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
			})
		})

		Convey("Listing works", func() {
			inst := putInst("pkg", digest, nil)

			So(AttachMetadata(ctx, inst, []*api.InstanceMetadata{
				{Key: "a", Value: []byte("0")},
				{Key: "b", Value: []byte("0")},
				{Key: "c", Value: []byte("0")},
			}), ShouldBeNil)

			tc.Add(time.Second)
			So(AttachMetadata(ctx, inst, []*api.InstanceMetadata{
				{Key: "a", Value: []byte("1")},
				{Key: "b", Value: []byte("1")},
				{Key: "c", Value: []byte("1")},
			}), ShouldBeNil)

			tc.Add(time.Second)
			So(AttachMetadata(ctx, inst, []*api.InstanceMetadata{
				{Key: "a", Value: []byte("2")},
				{Key: "b", Value: []byte("2")},
				{Key: "c", Value: []byte("2")},
			}), ShouldBeNil)

			pairs := func(md []*InstanceMetadata) []string {
				var out []string
				for _, m := range md {
					out = append(out, m.Key+":"+string(m.Value))
				}
				return out
			}

			Convey("ListMetadata", func() {
				Convey("Some", func() {
					md, err := ListMetadata(ctx, inst)
					So(err, ShouldBeNil)
					So(pairs(md), ShouldResemble, []string{
						"a:2", "b:2", "c:2", "a:1", "b:1", "c:1", "a:0", "b:0", "c:0",
					})
				})
				Convey("None", func() {
					inst := putInst("another/pkg", digest, nil)
					md, err := ListMetadata(ctx, inst)
					So(err, ShouldBeNil)
					So(md, ShouldHaveLength, 0)
				})
			})

			Convey("ListMetadataWithKeys", func() {
				Convey("Some", func() {
					md, err := ListMetadataWithKeys(ctx, inst, []string{"a", "c", "missing"})
					So(err, ShouldBeNil)
					So(pairs(md), ShouldResemble, []string{
						"a:2", "c:2", "a:1", "c:1", "a:0", "c:0",
					})
				})
				Convey("None", func() {
					md, err := ListMetadataWithKeys(ctx, inst, []string{"missing"})
					So(err, ShouldBeNil)
					So(md, ShouldHaveLength, 0)
				})
			})
		})
	})
}

func TestGuessPlainText(t *testing.T) {
	t.Parallel()

	Convey("guessPlainText works", t, func() {
		So(guessPlainText([]byte("")), ShouldBeTrue)
		So(guessPlainText([]byte("abc")), ShouldBeTrue)
		So(guessPlainText([]byte(" ~\t\r\n")), ShouldBeTrue)
		So(guessPlainText([]byte{0x19}), ShouldBeFalse)
		So(guessPlainText([]byte{0x7F}), ShouldBeFalse)
	})
}
