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

	"google.golang.org/grpc/codes"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcutil"

	api "go.chromium.org/luci/cipd/api/cipd/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestResolveVersion(t *testing.T) {
	t.Parallel()

	Convey("With datastore", t, func() {
		ctx, _, _ := TestingContext()

		pkg := &Package{Name: "pkg"}
		inst1 := &Instance{
			InstanceID:   strings.Repeat("1", 40),
			Package:      PackageKey(ctx, "pkg"),
			RegisteredBy: "user:1@example.com",
		}
		inst2 := &Instance{
			InstanceID:   strings.Repeat("2", 40),
			Package:      PackageKey(ctx, "pkg"),
			RegisteredBy: "user:2@example.com",
		}
		missing := &Instance{
			InstanceID:   strings.Repeat("3", 40),
			Package:      PackageKey(ctx, "pkg"),
			RegisteredBy: "user:3@example.com",
		}

		So(datastore.Put(ctx,
			pkg, inst1, inst2, // note: do not store 'missing' here
			&Ref{
				Name:       "latest",
				Package:    PackageKey(ctx, "pkg"),
				InstanceID: inst2.InstanceID,
			},
			&Ref{
				Name:       "broken",
				Package:    PackageKey(ctx, "pkg"),
				InstanceID: missing.InstanceID,
			},
			&Tag{
				ID:       TagID(&api.Tag{Key: "ver", Value: "1"}),
				Instance: datastore.KeyForObj(ctx, inst1),
				Tag:      "ver:1",
			},
			&Tag{
				ID:       TagID(&api.Tag{Key: "ver", Value: "ambiguous"}),
				Instance: datastore.KeyForObj(ctx, inst1),
				Tag:      "ver:ambiguous",
			},
			&Tag{
				ID:       TagID(&api.Tag{Key: "ver", Value: "ambiguous"}),
				Instance: datastore.KeyForObj(ctx, inst2),
				Tag:      "ver:ambiguous",
			},
			&Tag{
				ID:       TagID(&api.Tag{Key: "ver", Value: "broken"}),
				Instance: datastore.KeyForObj(ctx, missing),
				Tag:      "ver:broken",
			},
		), ShouldBeNil)

		Convey("Resolves instance ID", func() {
			inst, err := ResolveVersion(ctx, "pkg", inst1.InstanceID)
			So(err, ShouldBeNil)
			So(inst, ShouldResemble, inst1)
		})

		Convey("Resolves ref", func() {
			inst, err := ResolveVersion(ctx, "pkg", "latest")
			So(err, ShouldBeNil)
			So(inst, ShouldResemble, inst2)
		})

		Convey("Resolves tag", func() {
			inst, err := ResolveVersion(ctx, "pkg", "ver:1")
			So(err, ShouldBeNil)
			So(inst, ShouldResemble, inst1)
		})

		Convey("Fails on unrecognized version format", func() {
			_, err := ResolveVersion(ctx, "pkg", "::::")
			So(grpcutil.Code(err), ShouldEqual, codes.InvalidArgument)
			So(err, ShouldErrLike, "not a valid version identifier")
		})

		Convey("Handles missing instance", func() {
			Convey("Via instance ID", func() {
				_, err := ResolveVersion(ctx, "pkg", missing.InstanceID)
				So(grpcutil.Code(err), ShouldEqual, codes.NotFound)
				So(err, ShouldErrLike, "no such instance")
			})
			Convey("Via broken ref", func() {
				_, err := ResolveVersion(ctx, "pkg", "broken")
				So(grpcutil.Code(err), ShouldEqual, codes.NotFound)
				So(err, ShouldErrLike, "no such instance")
			})
			Convey("Via broken tag", func() {
				_, err := ResolveVersion(ctx, "pkg", "ver:broken")
				So(grpcutil.Code(err), ShouldEqual, codes.NotFound)
				So(err, ShouldErrLike, "no such instance")
			})
		})

		Convey("Handles missing package", func() {
			Convey("Via instance ID", func() {
				_, err := ResolveVersion(ctx, "pkg2", inst1.InstanceID)
				So(grpcutil.Code(err), ShouldEqual, codes.NotFound)
				So(err, ShouldErrLike, "no such package")
			})
			Convey("Via ref", func() {
				_, err := ResolveVersion(ctx, "pkg2", "latest")
				So(grpcutil.Code(err), ShouldEqual, codes.NotFound)
				So(err, ShouldErrLike, "no such package")
			})
			Convey("Via tag", func() {
				_, err := ResolveVersion(ctx, "pkg2", "ver:1")
				So(grpcutil.Code(err), ShouldEqual, codes.NotFound)
				So(err, ShouldErrLike, "no such package")
			})
		})

		Convey("Missing ref", func() {
			_, err := ResolveVersion(ctx, "pkg", "missing")
			So(grpcutil.Code(err), ShouldEqual, codes.NotFound)
			So(err, ShouldErrLike, "no such ref")
		})

		Convey("Missing tag", func() {
			_, err := ResolveVersion(ctx, "pkg", "ver:missing")
			So(grpcutil.Code(err), ShouldEqual, codes.NotFound)
			So(err, ShouldErrLike, "no such tag")
		})

		Convey("Ambiguous tag", func() {
			_, err := ResolveVersion(ctx, "pkg", "ver:ambiguous")
			So(grpcutil.Code(err), ShouldEqual, codes.FailedPrecondition)
			So(err, ShouldErrLike, "ambiguity when resolving the tag")
		})
	})
}
