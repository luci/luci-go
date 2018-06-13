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
	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/proto/google"
	"go.chromium.org/luci/grpc/grpcutil"

	api "go.chromium.org/luci/cipd/api/cipd/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestRefs(t *testing.T) {
	t.Parallel()

	Convey("With datastore", t, func() {
		digest := strings.Repeat("a", 40)

		testTime := testclock.TestRecentTimeUTC.Round(time.Millisecond)
		ctx, _ := testclock.UseTime(gaetesting.TestingContext(), testTime)

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
			putInst("pkg", digest, nil)

			// Missing initially.
			ref, err := GetRef(ctx, "pkg", "latest")
			So(grpcutil.Code(err), ShouldEqual, codes.NotFound)
			So(err, ShouldErrLike, "no such ref")

			// Create.
			So(SetRef(ctx, "latest", &Instance{
				InstanceID: digest,
				Package:    PackageKey(ctx, "pkg"),
			}, "user:abc@example.com"), ShouldBeNil)

			// Exists now.
			ref, err = GetRef(ctx, "pkg", "latest")
			So(err, ShouldBeNil)
			So(ref.Proto(), ShouldResembleProto, &api.Ref{
				Name:    "latest",
				Package: "pkg",
				Instance: &api.ObjectRef{
					HashAlgo:  api.HashAlgo_SHA1,
					HexDigest: digest,
				},
				ModifiedBy: "user:abc@example.com",
				ModifiedTs: google.NewTimestamp(testTime),
			})

			// Delete.
			So(DeleteRef(ctx, "pkg", "latest"), ShouldBeNil)

			// Missing now.
			ref, err = GetRef(ctx, "pkg", "latest")
			So(grpcutil.Code(err), ShouldEqual, codes.NotFound)
			So(err, ShouldErrLike, "no such ref")
		})

		Convey("Instance not ready", func() {
			putInst("pkg", digest, []string{"proc"})

			err := SetRef(ctx, "latest", &Instance{
				InstanceID: digest,
				Package:    PackageKey(ctx, "pkg"),
			}, "user:abc@example.com")
			So(grpcutil.Code(err), ShouldEqual, codes.FailedPrecondition)
			So(err, ShouldErrLike, "the instance is not ready yet")
		})

		Convey("Doesn't touch existing ref", func() {
			putInst("pkg", digest, nil)

			So(SetRef(ctx, "latest", &Instance{
				InstanceID: digest,
				Package:    PackageKey(ctx, "pkg"),
			}, "user:abc@example.com"), ShouldBeNil)

			So(SetRef(ctx, "latest", &Instance{
				InstanceID: digest,
				Package:    PackageKey(ctx, "pkg"),
			}, "user:another@example.com"), ShouldBeNil)

			ref, err := GetRef(ctx, "pkg", "latest")
			So(err, ShouldBeNil)
			So(ref.ModifiedBy, ShouldEqual, "user:abc@example.com") // the initial one
		})
	})
}
