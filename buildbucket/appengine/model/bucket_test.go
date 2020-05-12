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

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"

	. "github.com/smartystreets/goconvey/convey"
)

func TestBucket(t *testing.T) {
	t.Parallel()

	Convey("Bucket", t, func() {
		s := &authtest.FakeState{}
		ctx := auth.WithState(memory.Use(context.Background()), s)
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)

		Convey("GetRole", func() {
			Convey("empty", func() {
				b := &Bucket{
					Proto: buildbucketpb.Bucket{},
				}
				r, err := b.GetRole(ctx)
				So(err, ShouldBeNil)
				So(r, ShouldBeNil)
			})

			Convey("project", func() {
				b := &Bucket{
					Parent: datastore.KeyForObj(ctx, &Project{
						ID: "project",
					}),
					Proto: buildbucketpb.Bucket{},
				}

				Convey("match", func() {
					s.Identity = identity.Identity("project:project")
					r, err := b.GetRole(ctx)
					So(err, ShouldBeNil)
					So(r, ShouldNotBeNil)
					So(*r, ShouldEqual, buildbucketpb.Acl_WRITER)
				})

				Convey("mismatch", func() {
					s.Identity = identity.Identity("project:other")
					r, err := b.GetRole(ctx)
					So(err, ShouldBeNil)
					So(r, ShouldBeNil)
				})
			})

			Convey("email", func() {
				b := &Bucket{
					Proto: buildbucketpb.Bucket{
						Acls: []*buildbucketpb.Acl{
							{
								Identity: "email1",
								Role:     buildbucketpb.Acl_READER,
							},
							{
								Identity: "email2",
								Role:     buildbucketpb.Acl_SCHEDULER,
							},
						},
					},
				}

				Convey("match", func() {
					s.Identity = identity.Identity("user:email1")
					r, err := b.GetRole(ctx)
					So(err, ShouldBeNil)
					So(r, ShouldNotBeNil)
					So(*r, ShouldEqual, buildbucketpb.Acl_READER)
				})

				Convey("mismatch", func() {
					s.Identity = identity.Identity("user:email3")
					r, err := b.GetRole(ctx)
					So(err, ShouldBeNil)
					So(r, ShouldBeNil)
				})
			})

			Convey("user", func() {
				b := &Bucket{
					Proto: buildbucketpb.Bucket{
						Acls: []*buildbucketpb.Acl{
							{
								Identity: "user:user1",
								Role:     buildbucketpb.Acl_SCHEDULER,
							},
							{
								Identity: "user:user2",
								Role:     buildbucketpb.Acl_WRITER,
							},
						},
					},
				}

				Convey("match", func() {
					s.Identity = identity.Identity("user:user2")
					r, err := b.GetRole(ctx)
					So(err, ShouldBeNil)
					So(r, ShouldNotBeNil)
					So(*r, ShouldEqual, buildbucketpb.Acl_WRITER)
				})

				Convey("mismatch", func() {
					s.Identity = identity.Identity("user:user3")
					r, err := b.GetRole(ctx)
					So(err, ShouldBeNil)
					So(r, ShouldBeNil)
				})
			})

			Convey("group", func() {
				b := &Bucket{
					Proto: buildbucketpb.Bucket{
						Acls: []*buildbucketpb.Acl{
							{
								Group: "group:group",
								Role:  buildbucketpb.Acl_READER,
							},
						},
					},
				}

				Convey("match", func() {
					s.IdentityGroups = []string{"group:group"}
					r, err := b.GetRole(ctx)
					So(err, ShouldBeNil)
					So(r, ShouldNotBeNil)
					So(*r, ShouldEqual, buildbucketpb.Acl_READER)
				})

				Convey("mismatch", func() {
					r, err := b.GetRole(ctx)
					So(err, ShouldBeNil)
					So(r, ShouldBeNil)
				})
			})
		})
	})
}
