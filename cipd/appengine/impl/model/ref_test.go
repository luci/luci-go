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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcutil"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
	"go.chromium.org/luci/cipd/appengine/impl/testutil"
)

func TestRefs(t *testing.T) {
	t.Parallel()

	ftt.Run("With datastore", t, func(t *ftt.Test) {
		digestA := strings.Repeat("a", 40)
		digestB := strings.Repeat("b", 40)

		ctx, tc, as := testutil.TestingContext()

		putInst := func(pkg, iid string, pendingProcs []string) {
			assert.Loosely(t, datastore.Put(ctx,
				&Package{Name: pkg},
				&Instance{
					InstanceID:        iid,
					Package:           PackageKey(ctx, pkg),
					ProcessorsPending: pendingProcs,
				}), should.BeNil)
		}

		t.Run("SetRef+GetRef+DeleteRef happy path", func(t *ftt.Test) {
			putInst("pkg", digestA, nil)
			putInst("pkg", digestB, nil)

			// Missing initially.
			ref, err := GetRef(ctx, "pkg", "latest")
			assert.Loosely(t, grpcutil.Code(err), should.Equal(codes.NotFound))
			assert.Loosely(t, err, should.ErrLike("no such ref"))

			// Create.
			assert.Loosely(t, SetRef(ctx, "latest", &Instance{
				InstanceID: digestA,
				Package:    PackageKey(ctx, "pkg"),
			}), should.BeNil)

			// Exists now.
			ref, err = GetRef(ctx, "pkg", "latest")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ref.Proto(), should.Resemble(&api.Ref{
				Name:    "latest",
				Package: "pkg",
				Instance: &api.ObjectRef{
					HashAlgo:  api.HashAlgo_SHA1,
					HexDigest: digestA,
				},
				ModifiedBy: string(testutil.TestUser),
				ModifiedTs: timestamppb.New(testutil.TestTime),
			}))

			// Move it to point to something else.
			tc.Add(time.Second)
			assert.Loosely(t, SetRef(ctx, "latest", &Instance{
				InstanceID: digestB,
				Package:    PackageKey(ctx, "pkg"),
			}), should.BeNil)
			ref, err = GetRef(ctx, "pkg", "latest")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ref.Proto().Instance.HexDigest, should.Equal(digestB))

			// Delete.
			tc.Add(time.Second)
			assert.Loosely(t, DeleteRef(ctx, "pkg", "latest"), should.BeNil)

			// Missing now.
			ref, err = GetRef(ctx, "pkg", "latest")
			assert.Loosely(t, grpcutil.Code(err), should.Equal(codes.NotFound))
			assert.Loosely(t, err, should.ErrLike("no such ref"))

			// Second delete is silent noop.
			tc.Add(time.Second)
			assert.Loosely(t, DeleteRef(ctx, "pkg", "latest"), should.BeNil)

			// Collected all events correctly.
			assert.Loosely(t, GetEvents(ctx), should.Resemble([]*api.Event{
				{
					Kind:     api.EventKind_INSTANCE_REF_UNSET,
					Package:  "pkg",
					Ref:      "latest",
					Instance: digestB,
					Who:      string(testutil.TestUser),
					When:     timestamppb.New(testutil.TestTime.Add(2 * time.Second)),
				},
				{
					Kind:     api.EventKind_INSTANCE_REF_SET,
					Package:  "pkg",
					Ref:      "latest",
					Instance: digestB,
					Who:      string(testutil.TestUser),
					When:     timestamppb.New(testutil.TestTime.Add(time.Second + 1)),
				},
				{
					Kind:     api.EventKind_INSTANCE_REF_UNSET,
					Package:  "pkg",
					Ref:      "latest",
					Instance: digestA,
					Who:      string(testutil.TestUser),
					When:     timestamppb.New(testutil.TestTime.Add(time.Second)),
				},
				{
					Kind:     api.EventKind_INSTANCE_REF_SET,
					Package:  "pkg",
					Ref:      "latest",
					Instance: digestA,
					Who:      string(testutil.TestUser),
					When:     timestamppb.New(testutil.TestTime),
				},
			}))
		})

		t.Run("Instance not ready", func(t *ftt.Test) {
			putInst("pkg", digestA, []string{"proc"})

			err := SetRef(ctx, "latest", &Instance{
				InstanceID: digestA,
				Package:    PackageKey(ctx, "pkg"),
			})
			assert.Loosely(t, grpcutil.Code(err), should.Equal(codes.FailedPrecondition))
			assert.Loosely(t, err, should.ErrLike("the instance is not ready yet"))
		})

		t.Run("Doesn't touch existing ref", func(t *ftt.Test) {
			putInst("pkg", digestA, nil)

			assert.Loosely(t, SetRef(ctx, "latest", &Instance{
				InstanceID: digestA,
				Package:    PackageKey(ctx, "pkg"),
			}), should.BeNil)

			assert.Loosely(t, SetRef(as("another@example.com"), "latest", &Instance{
				InstanceID: digestA,
				Package:    PackageKey(ctx, "pkg"),
			}), should.BeNil)

			ref, err := GetRef(ctx, "pkg", "latest")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ref.ModifiedBy, should.Equal(string(testutil.TestUser))) // the initial one
		})

		t.Run("ListPackageRefs works", func(t *ftt.Test) {
			putInst("pkg", digestA, nil)
			pkgKey := PackageKey(ctx, "pkg")

			assert.Loosely(t, SetRef(ctx, "ref-0", &Instance{
				InstanceID: digestA,
				Package:    pkgKey,
			}), should.BeNil)

			tc.Add(time.Minute)

			assert.Loosely(t, SetRef(ctx, "ref-1", &Instance{
				InstanceID: digestA,
				Package:    pkgKey,
			}), should.BeNil)

			refs, err := ListPackageRefs(ctx, "pkg")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, refs, should.Resemble([]*Ref{
				{
					Name:       "ref-1",
					Package:    pkgKey,
					InstanceID: digestA,
					ModifiedBy: string(testutil.TestUser),
					ModifiedTs: testutil.TestTime.Add(time.Minute),
				},
				{
					Name:       "ref-0",
					Package:    pkgKey,
					InstanceID: digestA,
					ModifiedBy: string(testutil.TestUser),
					ModifiedTs: testutil.TestTime,
				},
			}))
		})

		t.Run("ListInstanceRefs works", func(t *ftt.Test) {
			pkgKey := PackageKey(ctx, "pkg")

			inst1 := &Instance{
				InstanceID: strings.Repeat("a", 40),
				Package:    pkgKey,
			}
			putInst("pkg", inst1.InstanceID, nil)

			inst2 := &Instance{
				InstanceID: strings.Repeat("b", 40),
				Package:    pkgKey,
			}
			putInst("pkg", inst2.InstanceID, nil)

			assert.Loosely(t, SetRef(ctx, "ref-0", inst1), should.BeNil)
			tc.Add(time.Minute)
			assert.Loosely(t, SetRef(ctx, "ref-1", inst1), should.BeNil)
			assert.Loosely(t, SetRef(ctx, "another-ref", inst2), should.BeNil)

			refs, err := ListInstanceRefs(ctx, inst1)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, refs, should.Resemble([]*Ref{
				{
					Name:       "ref-1",
					Package:    pkgKey,
					InstanceID: inst1.InstanceID,
					ModifiedBy: string(testutil.TestUser),
					ModifiedTs: testutil.TestTime.Add(time.Minute),
				},
				{
					Name:       "ref-0",
					Package:    pkgKey,
					InstanceID: inst1.InstanceID,
					ModifiedBy: string(testutil.TestUser),
					ModifiedTs: testutil.TestTime,
				},
			}))
		})
	})
}
