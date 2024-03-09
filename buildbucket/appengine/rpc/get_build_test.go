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

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/buildbucket/appengine/model"
	"go.chromium.org/luci/buildbucket/appengine/rpc/testutil"
	"go.chromium.org/luci/buildbucket/bbperms"
	pb "go.chromium.org/luci/buildbucket/proto"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestGetBuild(t *testing.T) {
	t.Parallel()

	const userID = identity.Identity("user:user@example.com")

	Convey("GetBuild", t, func() {
		srv := &Builds{}
		ctx := memory.Use(context.Background())
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)

		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity: userID,
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

			Convey("with build entity", func() {
				testutil.PutBucket(ctx, "project", "bucket", nil)
				build := &model.Build{
					Proto: &pb.Build{
						Id: 1,
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
						Input: &pb.Build_Input{
							GerritChanges: []*pb.GerritChange{
								{Host: "h1"},
								{Host: "h2"},
							},
						},
						CancellationMarkdown: "cancelled",
						SummaryMarkdown:      "summary",
					},
				}
				So(datastore.Put(ctx, build), ShouldBeNil)
				key := datastore.KeyForObj(ctx, build)
				s, err := proto.Marshal(&pb.Build{
					Steps: []*pb.Step{
						{
							Name: "step",
						},
					},
				})
				So(err, ShouldBeNil)
				So(datastore.Put(ctx, &model.BuildSteps{
					Build:    key,
					Bytes:    s,
					IsZipped: false,
				}), ShouldBeNil)
				So(datastore.Put(ctx, &model.BuildInfra{
					Build: key,
					Proto: &pb.BuildInfra{
						Buildbucket: &pb.BuildInfra_Buildbucket{
							Hostname: "example.com",
						},
						Resultdb: &pb.BuildInfra_ResultDB{
							Hostname:   "rdb.example.com",
							Invocation: "bb-12345",
						},
					},
				}), ShouldBeNil)
				So(datastore.Put(ctx, &model.BuildInputProperties{
					Build: key,
					Proto: &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"input": {
								Kind: &structpb.Value_StringValue{
									StringValue: "input value",
								},
							},
						},
					},
				}), ShouldBeNil)
				So(datastore.Put(ctx, &model.BuildOutputProperties{
					Build: key,
					Proto: &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"output": {
								Kind: &structpb.Value_StringValue{
									StringValue: "output value",
								},
							},
						},
					},
				}), ShouldBeNil)

				req := &pb.GetBuildRequest{
					Id: 1,
					Mask: &pb.BuildMask{
						AllFields: true,
					},
				}

				Convey("permission denied", func() {
					rsp, err := srv.GetBuild(ctx, req)
					So(err, ShouldErrLike, "not found")
					So(rsp, ShouldBeNil)
				})

				Convey("permission denied if user only has BuildsList permission", func() {
					ctx = auth.WithState(ctx, &authtest.FakeState{
						Identity: userID,
						FakeDB: authtest.NewFakeDB(
							authtest.MockPermission(userID, "project:bucket", bbperms.BuildersList),
							authtest.MockPermission(userID, "project:bucket", bbperms.BuildsList),
						),
					})
					rsp, err := srv.GetBuild(ctx, req)
					So(err, ShouldErrLike, "not found")
					So(rsp, ShouldBeNil)
				})

				Convey("found with BuildsGetLimited permission only", func() {
					ctx = auth.WithState(ctx, &authtest.FakeState{
						Identity: userID,
						FakeDB: authtest.NewFakeDB(
							authtest.MockPermission(userID, "project:bucket", bbperms.BuildersList),
							authtest.MockPermission(userID, "project:bucket", bbperms.BuildsGetLimited),
						),
					})
					rsp, err := srv.GetBuild(ctx, req)
					So(err, ShouldBeNil)
					So(rsp, ShouldResembleProto, &pb.Build{
						Id: 1,
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
						Input: &pb.Build_Input{
							GerritChanges: []*pb.GerritChange{
								{Host: "h1"},
								{Host: "h2"},
							},
						},
						Infra: &pb.BuildInfra{
							Resultdb: &pb.BuildInfra_ResultDB{
								Hostname:   "rdb.example.com",
								Invocation: "bb-12345",
							},
						},
					})
				})

				Convey("found", func() {
					ctx = auth.WithState(ctx, &authtest.FakeState{
						Identity: userID,
						FakeDB: authtest.NewFakeDB(
							authtest.MockPermission(userID, "project:bucket", bbperms.BuildsGet),
						),
					})
					rsp, err := srv.GetBuild(ctx, req)
					So(err, ShouldBeNil)
					So(rsp, ShouldResembleProto, &pb.Build{
						Id: 1,
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
						Input: &pb.Build_Input{
							Properties: &structpb.Struct{
								Fields: map[string]*structpb.Value{
									"input": {
										Kind: &structpb.Value_StringValue{
											StringValue: "input value",
										},
									},
								},
							},
							GerritChanges: []*pb.GerritChange{
								{Host: "h1"},
								{Host: "h2"},
							},
						},
						Output: &pb.Build_Output{
							Properties: &structpb.Struct{
								Fields: map[string]*structpb.Value{
									"output": {
										Kind: &structpb.Value_StringValue{
											StringValue: "output value",
										},
									},
								},
							},
						},
						Infra: &pb.BuildInfra{
							Buildbucket: &pb.BuildInfra_Buildbucket{
								Hostname: "example.com",
							},
							Resultdb: &pb.BuildInfra_ResultDB{
								Hostname:   "rdb.example.com",
								Invocation: "bb-12345",
							},
						},
						Steps: []*pb.Step{
							{Name: "step"},
						},
						CancellationMarkdown: "cancelled",
						SummaryMarkdown:      "summary\ncancelled",
					})
				})

				Convey("summary", func() {
					ctx = auth.WithState(ctx, &authtest.FakeState{
						Identity: userID,
						FakeDB: authtest.NewFakeDB(
							authtest.MockPermission(userID, "project:bucket", bbperms.BuildsGet),
						),
					})
					req.Mask = &pb.BuildMask{
						Fields: &fieldmaskpb.FieldMask{
							Paths: []string{
								"summary_markdown",
							},
						},
					}
					rsp, err := srv.GetBuild(ctx, req)
					So(err, ShouldBeNil)
					So(rsp, ShouldResembleProto, &pb.Build{
						SummaryMarkdown: "summary\ncancelled",
					})
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
								BuildID:  1,
								BucketID: "proj/bucket",
							},
							{
								BuildID:  2,
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
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity: userID,
					FakeDB: authtest.NewFakeDB(
						authtest.MockPermission(userID, "project:bucket", bbperms.BuildsGet),
					),
				})
				testutil.PutBucket(ctx, "project", "bucket", nil)
				So(datastore.Put(ctx, &model.TagIndex{
					ID: ":2:build_address:luci.project.bucket/builder/1",
					Entries: []model.TagIndexEntry{
						{
							BuildID:  1,
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

		Convey("led build", func() {
			testutil.PutBucket(ctx, "project", "bucket", nil)
			testutil.PutBucket(ctx, "project", "bucket.shadow", nil)
			build := &model.Build{
				Proto: &pb.Build{
					Id: 1,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket.shadow",
						Builder: "builder",
					},
					Input: &pb.Build_Input{
						GerritChanges: []*pb.GerritChange{
							{Host: "h1"},
							{Host: "h2"},
						},
					},
					CancellationMarkdown: "cancelled",
					SummaryMarkdown:      "summary",
				},
			}
			So(datastore.Put(ctx, build), ShouldBeNil)
			key := datastore.KeyForObj(ctx, build)
			s, err := proto.Marshal(&pb.Build{
				Steps: []*pb.Step{
					{
						Name: "step",
					},
				},
			})
			So(err, ShouldBeNil)
			So(datastore.Put(ctx, &model.BuildSteps{
				Build:    key,
				Bytes:    s,
				IsZipped: false,
			}), ShouldBeNil)
			So(datastore.Put(ctx, &model.BuildInfra{
				Build: key,
				Proto: &pb.BuildInfra{
					Buildbucket: &pb.BuildInfra_Buildbucket{
						Hostname: "example.com",
					},
					Resultdb: &pb.BuildInfra_ResultDB{
						Hostname:   "rdb.example.com",
						Invocation: "bb-12345",
					},
					Led: &pb.BuildInfra_Led{
						ShadowedBucket: "bucket",
					},
				},
			}), ShouldBeNil)
			So(datastore.Put(ctx, &model.BuildInputProperties{
				Build: key,
				Proto: &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"input": {
							Kind: &structpb.Value_StringValue{
								StringValue: "input value",
							},
						},
					},
				},
			}), ShouldBeNil)
			So(datastore.Put(ctx, &model.BuildOutputProperties{
				Build: key,
				Proto: &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"output": {
							Kind: &structpb.Value_StringValue{
								StringValue: "output value",
							},
						},
					},
				},
			}), ShouldBeNil)

			req := &pb.GetBuildRequest{
				Id: 1,
				Mask: &pb.BuildMask{
					AllFields: true,
				},
			}

			Convey("permission denied", func() {
				rsp, err := srv.GetBuild(ctx, req)
				So(err, ShouldErrLike, "not found")
				So(rsp, ShouldBeNil)
			})

			Convey("found with permission on shadowed bucket", func() {
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity: userID,
					FakeDB: authtest.NewFakeDB(
						authtest.MockPermission(userID, "project:bucket", bbperms.BuildsGet),
					),
				})
				rsp, err := srv.GetBuild(ctx, req)
				So(err, ShouldBeNil)
				So(rsp, ShouldResembleProto, &pb.Build{
					Id: 1,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket.shadow",
						Builder: "builder",
					},
					Input: &pb.Build_Input{
						Properties: &structpb.Struct{
							Fields: map[string]*structpb.Value{
								"input": {
									Kind: &structpb.Value_StringValue{
										StringValue: "input value",
									},
								},
							},
						},
						GerritChanges: []*pb.GerritChange{
							{Host: "h1"},
							{Host: "h2"},
						},
					},
					Output: &pb.Build_Output{
						Properties: &structpb.Struct{
							Fields: map[string]*structpb.Value{
								"output": {
									Kind: &structpb.Value_StringValue{
										StringValue: "output value",
									},
								},
							},
						},
					},
					Infra: &pb.BuildInfra{
						Buildbucket: &pb.BuildInfra_Buildbucket{
							Hostname: "example.com",
						},
						Resultdb: &pb.BuildInfra_ResultDB{
							Hostname:   "rdb.example.com",
							Invocation: "bb-12345",
						},
						Led: &pb.BuildInfra_Led{
							ShadowedBucket: "bucket",
						},
					},
					Steps: []*pb.Step{
						{Name: "step"},
					},
					CancellationMarkdown: "cancelled",
					SummaryMarkdown:      "summary\ncancelled",
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
