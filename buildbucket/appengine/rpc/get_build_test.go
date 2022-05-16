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

package rpc

import (
	"context"
	"testing"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	// TODO(crbug/1242998): Remove once safe get becomes datastore default.
	_ "go.chromium.org/luci/gae/service/datastore/crbug1242998safeget"

	"go.chromium.org/luci/buildbucket/appengine/model"
	pb "go.chromium.org/luci/buildbucket/proto"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestGetBuild(t *testing.T) {
	t.Parallel()

	Convey("GetBuild", t, func() {
		srv := &Builds{}
		ctx := memory.Use(context.Background())
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)

		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity: "user:caller@example.com",
		})

		Convey("id", func() {
			Convey("not found", func() {
				req := &pb.GetBuildRequest{
					Id: 1,
				}
				rsp, err := srv.GetBuild(ctx, req)
				So(err, ShouldErrLike, "not found")
				So(rsp, ShouldBeNil)
			})

			Convey("permission denied", func() {
				So(datastore.Put(ctx, &model.Build{
					Proto: &pb.Build{
						Id: 1,
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
					},
				}), ShouldBeNil)
				req := &pb.GetBuildRequest{
					Id: 1,
				}
				rsp, err := srv.GetBuild(ctx, req)
				So(err, ShouldErrLike, "not found")
				So(rsp, ShouldBeNil)
			})

			Convey("found", func() {
				So(datastore.Put(ctx, &model.Bucket{
					ID:     "bucket",
					Parent: model.ProjectKey(ctx, "project"),
					Proto: &pb.Bucket{
						Acls: []*pb.Acl{
							{
								Identity: "user:caller@example.com",
								Role:     pb.Acl_READER,
							},
						},
					},
				}), ShouldBeNil)
				So(datastore.Put(ctx, &model.Build{
					Proto: &pb.Build{
						Id: 1,
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
					},
				}), ShouldBeNil)
				req := &pb.GetBuildRequest{
					Id: 1,
				}
				rsp, err := srv.GetBuild(ctx, req)
				So(err, ShouldBeNil)
				So(rsp, ShouldResembleProto, &pb.Build{
					Id: 1,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					Input: &pb.Build_Input{},
				})
			})
		})

		Convey("index", func() {
			So(datastore.Put(ctx, &model.Build{
				Proto: &pb.Build{
					Id: 1,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
				},
				BucketID: "project/bucket",
				Tags:     []string{"build_address:luci.project.bucket/builder/1"},
			}), ShouldBeNil)

			Convey("error", func() {
				Convey("incomplete index", func() {
					So(datastore.Put(ctx, &model.TagIndex{
						ID: ":2:build_address:luci.project.bucket/builder/1",
						Entries: []model.TagIndexEntry{
							{
								BuildID: 1,
							},
						},
						Incomplete: true,
					}), ShouldBeNil)
					req := &pb.GetBuildRequest{
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
						BuildNumber: 1,
					}
					rsp, err := srv.GetBuild(ctx, req)
					So(err, ShouldErrLike, "unexpected incomplete index")
					So(rsp, ShouldBeNil)
				})

				Convey("not found", func() {
					req := &pb.GetBuildRequest{
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
						BuildNumber: 2,
					}
					rsp, err := srv.GetBuild(ctx, req)
					So(err, ShouldErrLike, "not found")
					So(rsp, ShouldBeNil)
				})

				Convey("excessive results", func() {
					So(datastore.Put(ctx, &model.TagIndex{
						ID: ":2:build_address:luci.project.bucket/builder/1",
						Entries: []model.TagIndexEntry{
							{
								BuildID: 1,
								BucketID: "proj/bucket",
							},
							{
								BuildID: 2,
								BucketID: "proj/bucket",
							},
						},
					}), ShouldBeNil)
					req := &pb.GetBuildRequest{
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
						BuildNumber: 1,
					}
					rsp, err := srv.GetBuild(ctx, req)
					So(err, ShouldErrLike, "unexpected number of results")
					So(rsp, ShouldBeNil)
				})
			})

			Convey("ok", func() {
				So(datastore.Put(ctx, &model.Bucket{
					ID:     "bucket",
					Parent: model.ProjectKey(ctx, "project"),
					Proto: &pb.Bucket{
						Acls: []*pb.Acl{
							{
								Identity: "user:caller@example.com",
							},
						},
					},
				}), ShouldBeNil)
				So(datastore.Put(ctx, &model.TagIndex{
					ID: ":2:build_address:luci.project.bucket/builder/1",
					Entries: []model.TagIndexEntry{
						{
							BuildID: 1,
							BucketID: "project/bucket",
						},
					},
				}), ShouldBeNil)
				req := &pb.GetBuildRequest{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					BuildNumber: 1,
				}
				rsp, err := srv.GetBuild(ctx, req)
				So(err, ShouldBeNil)
				So(rsp, ShouldResembleProto, &pb.Build{
					Id: 1,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					Input: &pb.Build_Input{},
				})
			})
		})
	})

	Convey("validateGet", t, func() {
		Convey("nil", func() {
			err := validateGet(nil)
			So(err, ShouldErrLike, "id or (builder and build_number) is required")
		})

		Convey("empty", func() {
			req := &pb.GetBuildRequest{}
			err := validateGet(req)
			So(err, ShouldErrLike, "id or (builder and build_number) is required")
		})

		Convey("builder", func() {
			req := &pb.GetBuildRequest{
				Builder: &pb.BuilderID{},
			}
			err := validateGet(req)
			So(err, ShouldErrLike, "id or (builder and build_number) is required")
		})

		Convey("build number", func() {
			req := &pb.GetBuildRequest{
				BuildNumber: 1,
			}
			err := validateGet(req)
			So(err, ShouldErrLike, "id or (builder and build_number) is required")
		})

		Convey("mutual exclusion", func() {
			Convey("builder", func() {
				req := &pb.GetBuildRequest{
					Id:      1,
					Builder: &pb.BuilderID{},
				}
				err := validateGet(req)
				So(err, ShouldErrLike, "id is mutually exclusive with (builder and build_number)")
			})

			Convey("build number", func() {
				req := &pb.GetBuildRequest{
					Id:          1,
					BuildNumber: 1,
				}
				err := validateGet(req)
				So(err, ShouldErrLike, "id is mutually exclusive with (builder and build_number)")
			})
		})

		Convey("builder ID", func() {
			Convey("project", func() {
				req := &pb.GetBuildRequest{
					Builder:     &pb.BuilderID{},
					BuildNumber: 1,
				}
				err := validateGet(req)
				So(err, ShouldErrLike, "project must match")
			})

			Convey("bucket", func() {
				Convey("empty", func() {
					req := &pb.GetBuildRequest{
						Builder: &pb.BuilderID{
							Project: "project",
						},
						BuildNumber: 1,
					}
					err := validateGet(req)
					So(err, ShouldErrLike, "bucket is required")
				})

				Convey("v1", func() {
					req := &pb.GetBuildRequest{
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "luci.project.bucket",
							Builder: "builder",
						},
						BuildNumber: 1,
					}
					err := validateGet(req)
					So(err, ShouldErrLike, "invalid use of v1 bucket in v2 API")
				})
			})

			Convey("builder", func() {
				req := &pb.GetBuildRequest{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
					},
					BuildNumber: 1,
				}
				err := validateGet(req)
				So(err, ShouldErrLike, "builder is required")
			})
		})
	})
}

func TestCategorizeCallerCrBug1186261(t *testing.T) {
	t.Parallel()

	Convey("categorizeCallerCrBug1186261", t, func() {
		run := func(s string) string {
			return categorizeCallerCrBug1186261(identity.Identity(s))
		}
		So(run("anonymous:anonymous"), ShouldResemble, "anonymous:anonymous")
		So(run("user:sa@some-project.iam.gserviceaccount.com"), ShouldResemble, "user:sa@some-project.iam.gserviceaccount.com")
		So(run("user:me@example.com"), ShouldResemble, "user:<other>")
		So(run("project:infra"), ShouldResemble, "project:infra")
		So(run("service:luci-cv"), ShouldResemble, "service:luci-cv")
	})
}
