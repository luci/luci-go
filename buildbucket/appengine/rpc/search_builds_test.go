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

	"go.chromium.org/gae/impl/memory"
	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/logging/memlogger"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/buildbucket/appengine/model"
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

	Convey("validateCommit", t, func() {
		Convey("nil", func() {
			err := validateCommit(nil)
			So(err, ShouldErrLike, "host is required")
		})

		Convey("empty", func() {
			cm := &pb.GitilesCommit{}
			err := validateCommit(cm)
			So(err, ShouldErrLike, "host is required")
		})

		Convey("project", func() {
			cm := &pb.GitilesCommit{
				Host: "host",
			}
			err := validateCommit(cm)
			So(err, ShouldErrLike, "project is required")
		})

		Convey("id", func() {
			cm := &pb.GitilesCommit{
				Host:    "host",
				Project: "project",
				Id:      "id",
			}
			err := validateCommit(cm)
			So(err, ShouldErrLike, "id must match")
		})

		Convey("ref", func() {
			cm := &pb.GitilesCommit{
				Host:    "host",
				Project: "project",
				Ref:     "ref",
			}
			err := validateCommit(cm)
			So(err, ShouldErrLike, "ref must match")
		})

		Convey("mutual exclusion", func() {
			Convey("ref", func() {
				cm := &pb.GitilesCommit{
					Host:    "host",
					Project: "project",
					Id:      "id",
					Ref:     "ref",
				}
				err := validateCommit(cm)
				So(err, ShouldErrLike, "id is mutually exclusive with (ref and position)")
			})

			Convey("position", func() {
				cm := &pb.GitilesCommit{
					Host:     "host",
					Project:  "project",
					Id:       "id",
					Position: 1,
				}
				err := validateCommit(cm)
				So(err, ShouldErrLike, "id is mutually exclusive with (ref and position)")
			})

			Convey("neither", func() {
				cm := &pb.GitilesCommit{
					Host:    "host",
					Project: "project",
				}
				err := validateCommit(cm)
				So(err, ShouldErrLike, "one of")
			})
		})

		Convey("valid", func() {
			Convey("id", func() {
				cm := &pb.GitilesCommit{
					Host:    "host",
					Project: "project",
					Id:      "1234567890123456789012345678901234567890",
				}
				err := validateCommit(cm)
				So(err, ShouldBeNil)
			})

			Convey("ref", func() {
				cm := &pb.GitilesCommit{
					Host:     "host",
					Project:  "project",
					Ref:      "refs/ref",
					Position: 1,
				}
				err := validateCommit(cm)
				So(err, ShouldBeNil)
			})
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
	})

	Convey("validatePageToken", t, func() {
		Convey("empty token", func() {
			err := validatePageToken("", []*pb.StringPair{})
			So(err, ShouldBeNil)
		})

		Convey("non TagIndex token", func() {
			err := validatePageToken("abc", []*pb.StringPair{})
			So(err, ShouldBeNil)
		})

		Convey("TagIndex token with indexed tags", func() {
			tags := []*pb.StringPair{
				{Key: "buildset", Value: "b1"},
			}
			err := validatePageToken("id>123", tags)
			So(err, ShouldBeNil)
		})

		Convey("TagIndex token with non indexed tags", func() {
			tags := []*pb.StringPair{
				{Key: "k", Value: "v"},
			}
			err := validatePageToken("id>123", tags)
			So(err, ShouldErrLike, "invalid page_token")
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

	Convey("search builds", t, func() {
		srv := &Builds{}
		ctx := memory.Use(context.Background())
		ctx = memlogger.Use(ctx)
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)

		req := &pb.SearchBuildsRequest{
			Predicate: &pb.BuildPredicate{
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
			},
		}

		Convey("query search on Builds", func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: identity.Identity("user:user"),
			})
			So(datastore.Put(ctx, &model.Bucket{
				ID:     "bucket",
				Parent: model.ProjectKey(ctx, "project"),
				Proto: pb.Bucket{
					Acls: []*pb.Acl{
						{
							Identity: "user:user",
							Role:     pb.Acl_READER,
						},
					},
				},
			}), ShouldBeNil)
			So(datastore.Put(ctx, &model.Build{
				Proto: pb.Build{
					Id: 1,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
				},
				BuilderID: "project/bucket/builder",
				Tags:      []string{"k1:v1", "k2:v2"},
			}), ShouldBeNil)
			So(datastore.Put(ctx, &model.Build{
				Proto: pb.Build{
					Id: 1,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder2",
					},
				},
				BuilderID: "project/bucket/builder2",
			}), ShouldBeNil)
			req.Predicate.Tags = []*pb.StringPair{
				{Key: "k1", Value: "v1"},
				{Key: "k2", Value: "v2"},
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
						Tags: []*pb.StringPair{
							{
								Key:   "k1",
								Value: "v1",
							},
							{
								Key:   "k2",
								Value: "v2",
							},
						},
					},
				},
			}
			So(rsp, ShouldResembleProto, expectedRsp)
		})
	})
}
