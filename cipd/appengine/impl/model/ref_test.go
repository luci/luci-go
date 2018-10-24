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
	"go.chromium.org/luci/common/proto/google"
	"go.chromium.org/luci/grpc/grpcutil"

	api "go.chromium.org/luci/cipd/api/cipd/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestRefs(t *testing.T) {
	t.Parallel()

	Convey("With datastore", t, func() {
		digestA := strings.Repeat("a", 40)
		digestB := strings.Repeat("b", 40)

		ctx, tc, as := TestingContext()

		putInst := func(pkg, iid string, pendingProcs []string) {
			So(datastore.Put(ctx,
				&Package{Name: pkg},
				&Instance{
					InstanceID:        iid,
					Package:           PackageKey(ctx, pkg),
					ProcessorsPending: pendingProcs,
				}), ShouldBeNil)
		}

		Convey("SetRef+GetRef+DeleteRef happy path", func() {
			putInst("pkg", digestA, nil)
			putInst("pkg", digestB, nil)

			// Missing initially.
			ref, err := GetRef(ctx, "pkg", "latest")
			So(grpcutil.Code(err), ShouldEqual, codes.NotFound)
			So(err, ShouldErrLike, "no such ref")

			// Create.
			So(SetRef(ctx, "latest", &Instance{
				InstanceID: digestA,
				Package:    PackageKey(ctx, "pkg"),
			}), ShouldBeNil)

			// Exists now.
			ref, err = GetRef(ctx, "pkg", "latest")
			So(err, ShouldBeNil)
			So(ref.Proto(), ShouldResembleProto, &api.Ref{
				Name:    "latest",
				Package: "pkg",
				Instance: &api.ObjectRef{
					HashAlgo:  api.HashAlgo_SHA1,
					HexDigest: digestA,
				},
				ModifiedBy: string(testUser),
				ModifiedTs: google.NewTimestamp(testTime),
			})

			// Move it to point to something else.
			tc.Add(time.Second)
			So(SetRef(ctx, "latest", &Instance{
				InstanceID: digestB,
				Package:    PackageKey(ctx, "pkg"),
			}), ShouldBeNil)
			ref, err = GetRef(ctx, "pkg", "latest")
			So(err, ShouldBeNil)
			So(ref.Proto().Instance.HexDigest, ShouldEqual, digestB)

			// Delete.
			tc.Add(time.Second)
			So(DeleteRef(ctx, "pkg", "latest"), ShouldBeNil)

			// Missing now.
			ref, err = GetRef(ctx, "pkg", "latest")
			So(grpcutil.Code(err), ShouldEqual, codes.NotFound)
			So(err, ShouldErrLike, "no such ref")

			// Second delete is silent noop.
			tc.Add(time.Second)
			So(DeleteRef(ctx, "pkg", "latest"), ShouldBeNil)

			// Collected all events correctly.
			So(GetEvents(ctx), ShouldResembleProto, []*api.Event{
				{
					Kind:     api.EventKind_INSTANCE_REF_UNSET,
					Package:  "pkg",
					Ref:      "latest",
					Instance: digestB,
					Who:      string(testUser),
					When:     google.NewTimestamp(testTime.Add(2 * time.Second)),
				},
				{
					Kind:     api.EventKind_INSTANCE_REF_SET,
					Package:  "pkg",
					Ref:      "latest",
					Instance: digestB,
					Who:      string(testUser),
					When:     google.NewTimestamp(testTime.Add(time.Second + 1)),
				},
				{
					Kind:     api.EventKind_INSTANCE_REF_UNSET,
					Package:  "pkg",
					Ref:      "latest",
					Instance: digestA,
					Who:      string(testUser),
					When:     google.NewTimestamp(testTime.Add(time.Second)),
				},
				{
					Kind:     api.EventKind_INSTANCE_REF_SET,
					Package:  "pkg",
					Ref:      "latest",
					Instance: digestA,
					Who:      string(testUser),
					When:     google.NewTimestamp(testTime),
				},
			})
		})

		Convey("Instance not ready", func() {
			putInst("pkg", digestA, []string{"proc"})

			err := SetRef(ctx, "latest", &Instance{
				InstanceID: digestA,
				Package:    PackageKey(ctx, "pkg"),
			})
			So(grpcutil.Code(err), ShouldEqual, codes.FailedPrecondition)
			So(err, ShouldErrLike, "the instance is not ready yet")
		})

		Convey("Doesn't touch existing ref", func() {
			putInst("pkg", digestA, nil)

			So(SetRef(ctx, "latest", &Instance{
				InstanceID: digestA,
				Package:    PackageKey(ctx, "pkg"),
			}), ShouldBeNil)

			So(SetRef(as("another@example.com"), "latest", &Instance{
				InstanceID: digestA,
				Package:    PackageKey(ctx, "pkg"),
			}), ShouldBeNil)

			ref, err := GetRef(ctx, "pkg", "latest")
			So(err, ShouldBeNil)
			So(ref.ModifiedBy, ShouldEqual, string(testUser)) // the initial one
		})

		Convey("ListPackageRefs works", func() {
			putInst("pkg", digestA, nil)
			pkgKey := PackageKey(ctx, "pkg")

			So(SetRef(ctx, "ref-0", &Instance{
				InstanceID: digestA,
				Package:    pkgKey,
			}), ShouldBeNil)

			tc.Add(time.Minute)

			So(SetRef(ctx, "ref-1", &Instance{
				InstanceID: digestA,
				Package:    pkgKey,
			}), ShouldBeNil)

			refs, err := ListPackageRefs(ctx, "pkg")
			So(err, ShouldBeNil)
			So(refs, ShouldResemble, []*Ref{
				{
					Name:       "ref-1",
					Package:    pkgKey,
					InstanceID: digestA,
					ModifiedBy: string(testUser),
					ModifiedTs: testTime.Add(time.Minute),
				},
				{
					Name:       "ref-0",
					Package:    pkgKey,
					InstanceID: digestA,
					ModifiedBy: string(testUser),
					ModifiedTs: testTime,
				},
			})
		})

		Convey("ListInstanceRefs works", func() {
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

			So(SetRef(ctx, "ref-0", inst1), ShouldBeNil)
			tc.Add(time.Minute)
			So(SetRef(ctx, "ref-1", inst1), ShouldBeNil)
			So(SetRef(ctx, "another-ref", inst2), ShouldBeNil)

			refs, err := ListInstanceRefs(ctx, inst1)
			So(err, ShouldBeNil)
			So(refs, ShouldResemble, []*Ref{
				{
					Name:       "ref-1",
					Package:    pkgKey,
					InstanceID: inst1.InstanceID,
					ModifiedBy: string(testUser),
					ModifiedTs: testTime.Add(time.Minute),
				},
				{
					Name:       "ref-0",
					Package:    pkgKey,
					InstanceID: inst1.InstanceID,
					ModifiedBy: string(testUser),
					ModifiedTs: testTime,
				},
			})
		})
	})
}
