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
	"context"
	"testing"

	"go.chromium.org/gae/impl/memory"
	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	pb "go.chromium.org/luci/buildbucket/proto"

	. "github.com/smartystreets/goconvey/convey"
)

func TestBucket(t *testing.T) {
	t.Parallel()

	Convey("Bucket", t, func() {
		s := &authtest.FakeState{}
		ctx := auth.WithState(memory.Use(context.Background()), s)
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)

		Convey("CanView", func() {
			Convey("empty", func() {
				b := &Bucket{
					Proto: pb.Bucket{},
				}
				can, err := b.CanView(ctx)
				So(err, ShouldBeNil)
				So(can, ShouldBeFalse)
			})

			Convey("project", func() {
				b := &Bucket{
					Parent: datastore.KeyForObj(ctx, &Project{
						ID: "project",
					}),
					Proto: pb.Bucket{},
				}

				Convey("can", func() {
					s.Identity = identity.Identity("project:project")
					can, err := b.CanView(ctx)
					So(err, ShouldBeNil)
					So(can, ShouldBeTrue)
				})

				Convey("cannot", func() {
					s.Identity = identity.Identity("project:other")
					can, err := b.CanView(ctx)
					So(err, ShouldBeNil)
					So(can, ShouldBeFalse)
				})
			})

			Convey("email", func() {
				b := &Bucket{
					Proto: pb.Bucket{
						Acls: []*pb.Acl{
							{
								Identity: "email1",
							},
							{
								Identity: "email2",
							},
						},
					},
				}

				Convey("can", func() {
					s.Identity = identity.Identity("user:email1")
					can, err := b.CanView(ctx)
					So(err, ShouldBeNil)
					So(can, ShouldBeTrue)
				})

				Convey("cannot", func() {
					s.Identity = identity.Identity("user:email3")
					can, err := b.CanView(ctx)
					So(err, ShouldBeNil)
					So(can, ShouldBeFalse)
				})
			})

			Convey("user", func() {
				b := &Bucket{
					Proto: pb.Bucket{
						Acls: []*pb.Acl{
							{
								Identity: "user:user1",
							},
							{
								Identity: "user:user2",
							},
						},
					},
				}

				Convey("can", func() {
					s.Identity = identity.Identity("user:user2")
					can, err := b.CanView(ctx)
					So(err, ShouldBeNil)
					So(can, ShouldBeTrue)
				})

				Convey("cannot", func() {
					s.Identity = identity.Identity("user:user3")
					can, err := b.CanView(ctx)
					So(err, ShouldBeNil)
					So(can, ShouldBeFalse)
				})
			})

			Convey("group", func() {
				b := &Bucket{
					Proto: pb.Bucket{
						Acls: []*pb.Acl{
							{
								Group: "group:group",
							},
						},
					},
				}

				Convey("can", func() {
					s.IdentityGroups = []string{"group:group"}
					can, err := b.CanView(ctx)
					So(err, ShouldBeNil)
					So(can, ShouldBeTrue)
				})

				Convey("cannot", func() {
					can, err := b.CanView(ctx)
					So(err, ShouldBeNil)
					So(can, ShouldBeFalse)
				})
			})
		})
	})
}
