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

	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/logging/memlogger"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	bb "go.chromium.org/luci/buildbucket"
	"go.chromium.org/luci/buildbucket/appengine/model"
	"go.chromium.org/luci/buildbucket/appengine/rpc/testutil"
	"go.chromium.org/luci/buildbucket/bbperms"
	pb "go.chromium.org/luci/buildbucket/proto"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestValidateSearchBuilds(t *testing.T) {
	t.Parallel()

	Convey("validateChange", t, func() {
		Convey("nil", func() {
			err := validateChange(nil)
			So(err, ShouldErrLike, "host is required")
		})

		Convey("empty", func() {
			ch := &pb.GerritChange{}
			err := validateChange(ch)
			So(err, ShouldErrLike, "host is required")
		})

		Convey("change", func() {
			ch := &pb.GerritChange{
				Host: "host",
			}
			err := validateChange(ch)
			So(err, ShouldErrLike, "change is required")
		})

		Convey("patchset", func() {
			ch := &pb.GerritChange{
				Host:   "host",
				Change: 1,
			}
			err := validateChange(ch)
			So(err, ShouldErrLike, "patchset is required")
		})

		Convey("valid", func() {
			ch := &pb.GerritChange{
				Host:     "host",
				Change:   1,
				Patchset: 1,
			}
			err := validateChange(ch)
			So(err, ShouldBeNil)
		})
	})

	Convey("validatePredicate", t, func() {
		Convey("nil", func() {
			err := validatePredicate(nil)
			So(err, ShouldBeNil)
		})

		Convey("empty", func() {
			pr := &pb.BuildPredicate{}
			err := validatePredicate(pr)
			So(err, ShouldBeNil)
		})

		Convey("mutual exclusion", func() {
			pr := &pb.BuildPredicate{
				Build:      &pb.BuildRange{},
				CreateTime: &pb.TimeRange{},
			}
			err := validatePredicate(pr)
			So(err, ShouldErrLike, "build is mutually exclusive with create_time")
		})

		Convey("builder id", func() {
			Convey("no project", func() {
				pr := &pb.BuildPredicate{
					Builder: &pb.BuilderID{Bucket: "bucket"},
				}
				err := validatePredicate(pr)
				So(err, ShouldErrLike, `builder: project must match "^[a-z0-9\\-_]+$"`)
			})
			Convey("only project and builder", func() {
				pr := &pb.BuildPredicate{
					Builder: &pb.BuilderID{
						Project: "project",
						Builder: "builder",
					},
				}
				err := validatePredicate(pr)
				So(err, ShouldErrLike, "builder: bucket is required")
			})
		})

		Convey("experiments", func() {
			Convey("empty", func() {
				pr := &pb.BuildPredicate{
					Experiments: []string{""},
				}
				err := validatePredicate(pr)
				So(err, ShouldErrLike, `too short (expected [+-]$experiment_name)`)
			})
			Convey("bang", func() {
				pr := &pb.BuildPredicate{
					Experiments: []string{"!something"},
				}
				err := validatePredicate(pr)
				So(err, ShouldErrLike, `first character must be + or -`)
			})
			Convey("canary conflict", func() {
				pr := &pb.BuildPredicate{
					Experiments: []string{"+" + bb.ExperimentBBCanarySoftware},
					Canary:      pb.Trinary_YES,
				}
				err := validatePredicate(pr)
				So(err, ShouldErrLike,
					`cannot specify "luci.buildbucket.canary_software" and canary in the same predicate`)
			})
			Convey("duplicate (bad)", func() {
				pr := &pb.BuildPredicate{
					Experiments: []string{
						"+" + bb.ExperimentBBCanarySoftware,
						"-" + bb.ExperimentBBCanarySoftware,
					},
				}
				err := validatePredicate(pr)
				So(err, ShouldErrLike,
					`"luci.buildbucket.canary_software" has both inclusive and exclusive filter`)
			})

			Convey("ok", func() {
				pr := &pb.BuildPredicate{
					Experiments: []string{
						"+" + bb.ExperimentBBCanarySoftware,
						"+" + bb.ExperimentNonProduction,
					},
				}
				So(validatePredicate(pr), ShouldBeNil)
			})
			Convey("duplicate (ok)", func() {
				pr := &pb.BuildPredicate{
					Experiments: []string{
						"+" + bb.ExperimentBBCanarySoftware,
						"+" + bb.ExperimentBBCanarySoftware,
					},
				}
				So(validatePredicate(pr), ShouldBeNil)
			})
		})

		Convey("descendant_of and child_of mutual exclusion", func() {
			pr := &pb.BuildPredicate{
				DescendantOf: 1,
				ChildOf:      1,
			}
			err := validatePredicate(pr)
			So(err, ShouldErrLike, "descendant_of is mutually exclusive with child_of")
		})
	})

	Convey("validatePageToken", t, func() {
		Convey("empty token", func() {
			err := validatePageToken("")
			So(err, ShouldBeNil)
		})

		Convey("invalid page token", func() {
			err := validatePageToken("abc")
			So(err, ShouldErrLike, "invalid page_token")
		})

		Convey("valid page token", func() {
			err := validatePageToken("id>123")
			So(err, ShouldBeNil)
		})
	})

	Convey("validateSearch", t, func() {
		Convey("nil", func() {
			err := validateSearch(nil)
			So(err, ShouldBeNil)
		})

		Convey("empty", func() {
			req := &pb.SearchBuildsRequest{}
			err := validateSearch(req)
			So(err, ShouldBeNil)
		})

		Convey("page size", func() {
			Convey("negative", func() {
				req := &pb.SearchBuildsRequest{
					PageSize: -1,
				}
				err := validateSearch(req)
				So(err, ShouldErrLike, "page_size cannot be negative")
			})

			Convey("zero", func() {
				req := &pb.SearchBuildsRequest{
					PageSize: 0,
				}
				err := validateSearch(req)
				So(err, ShouldBeNil)
			})

			Convey("positive", func() {
				req := &pb.SearchBuildsRequest{
					PageSize: 1,
				}
				err := validateSearch(req)
				So(err, ShouldBeNil)
			})
		})
	})
}

func TestSearchBuilds(t *testing.T) {
	t.Parallel()

	const userID = identity.Identity("user:user@example.com")

	Convey("search builds", t, func() {
		srv := &Builds{}
		ctx := memory.Use(context.Background())
		ctx = memlogger.Use(ctx)
		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity: userID,
			FakeDB: authtest.NewFakeDB(
				authtest.MockPermission(userID, "project:bucket", bbperms.BuildersList),
				authtest.MockPermission(userID, "project:bucket", bbperms.BuildsList),
				authtest.MockPermission(userID, "project:bucket", bbperms.BuildsGet),
			),
		})
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)
		testutil.PutBucket(ctx, "project", "bucket", nil)
		So(datastore.Put(ctx, &model.Build{
			Proto: &pb.Build{
				Id: 1,
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				SummaryMarkdown: "foo summary",
				Input: &pb.Build_Input{
					GerritChanges: []*pb.GerritChange{
						{Host: "h1"},
						{Host: "h2"},
					},
				},
			},
			BucketID:  "project/bucket",
			BuilderID: "project/bucket/builder",
			Tags:      []string{"k1:v1", "k2:v2"},
		}), ShouldBeNil)
		So(datastore.Put(ctx, &model.Build{
			Proto: &pb.Build{
				Id: 2,
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder2",
				},
			},
			BucketID:  "project/bucket",
			BuilderID: "project/bucket/builder2",
		}), ShouldBeNil)
		Convey("query search on Builds", func() {
			req := &pb.SearchBuildsRequest{
				Predicate: &pb.BuildPredicate{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					Tags: []*pb.StringPair{
						{Key: "k1", Value: "v1"},
						{Key: "k2", Value: "v2"},
					},
				},
			}
			rsp, err := srv.SearchBuilds(ctx, req)
			So(err, ShouldBeNil)
			expectedRsp := &pb.SearchBuildsResponse{
				Builds: []*pb.Build{
					{
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
					},
				},
			}
			So(rsp, ShouldResembleProto, expectedRsp)
		})

		Convey("search builds with field masks", func() {
			b := &model.Build{
				ID: 1,
			}
			key := datastore.KeyForObj(ctx, b)
			So(datastore.Put(ctx, &model.BuildInfra{
				Build: key,
				Proto: &pb.BuildInfra{
					Buildbucket: &pb.BuildInfra_Buildbucket{
						Hostname: "example.com",
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

			req := &pb.SearchBuildsRequest{
				Fields: &field_mask.FieldMask{
					Paths: []string{"builds.*.id", "builds.*.input", "builds.*.infra"},
				},
			}
			rsp, err := srv.SearchBuilds(ctx, req)
			So(err, ShouldBeNil)
			expectedRsp := &pb.SearchBuildsResponse{
				Builds: []*pb.Build{
					{
						Id: 1,
						Input: &pb.Build_Input{
							GerritChanges: []*pb.GerritChange{
								{Host: "h1"},
								{Host: "h2"},
							},
							Properties: &structpb.Struct{
								Fields: map[string]*structpb.Value{
									"input": {
										Kind: &structpb.Value_StringValue{
											StringValue: "input value",
										},
									},
								},
							},
						},
						Infra: &pb.BuildInfra{
							Buildbucket: &pb.BuildInfra_Buildbucket{
								Hostname: "example.com",
							},
						},
					},
					{
						Id:    2,
						Input: &pb.Build_Input{},
					},
				},
			}
			So(rsp, ShouldResembleProto, expectedRsp)
		})

		Convey("search builds with limited access", func() {
			key := datastore.KeyForObj(ctx, &model.Build{ID: 1})
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

			req := &pb.SearchBuildsRequest{
				Mask: &pb.BuildMask{
					AllFields: true,
				},
			}

			Convey("BuildsGetLimited only", func() {
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity: userID,
					FakeDB: authtest.NewFakeDB(
						authtest.MockPermission(userID, "project:bucket", bbperms.BuildersList),
						authtest.MockPermission(userID, "project:bucket", bbperms.BuildsGetLimited),
					),
				})

				rsp, err := srv.SearchBuilds(ctx, req)
				So(err, ShouldBeNil)
				expectedRsp := &pb.SearchBuildsResponse{
					Builds: []*pb.Build{
						{
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
						},
						{
							Id: 2,
							Builder: &pb.BuilderID{
								Project: "project",
								Bucket:  "bucket",
								Builder: "builder2",
							},
							Input: &pb.Build_Input{},
						},
					},
				}
				So(rsp, ShouldResembleProto, expectedRsp)
			})

			Convey("BuildsList only", func() {
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity: userID,
					FakeDB: authtest.NewFakeDB(
						authtest.MockPermission(userID, "project:bucket", bbperms.BuildersList),
						authtest.MockPermission(userID, "project:bucket", bbperms.BuildsList),
					),
				})

				rsp, err := srv.SearchBuilds(ctx, req)
				So(err, ShouldBeNil)
				expectedRsp := &pb.SearchBuildsResponse{
					Builds: []*pb.Build{
						{
							Id: 1,
						},
						{
							Id: 2,
						},
					},
				}
				So(rsp, ShouldResembleProto, expectedRsp)
			})
		})
	})
}
