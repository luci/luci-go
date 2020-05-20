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
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/buildbucket/appengine/model"
	pb "go.chromium.org/luci/buildbucket/proto"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestSearchBuilds(t *testing.T) {
	t.Parallel()

	Convey("validateSearch", t, func() {
		Convey("request", func() {
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

		Convey("predicate", func() {
			Convey("empty", func() {
				req := &pb.SearchBuildsRequest{
					Predicate: &pb.BuildPredicate{},
				}
				err := validateSearch(req)
				So(err, ShouldBeNil)
			})

			Convey("builder", func() {
				Convey("project", func() {
					req := &pb.SearchBuildsRequest{
						Predicate: &pb.BuildPredicate{
							Builder: &pb.BuilderID{},
						},
					}
					err := validateSearch(req)
					So(err, ShouldErrLike, "project must match")
				})

				Convey("bucket", func() {
					Convey("empty", func() {
						req := &pb.SearchBuildsRequest{
							Predicate: &pb.BuildPredicate{
								Builder: &pb.BuilderID{
									Project: "project",
								},
							},
						}
						err := validateSearch(req)
						So(err, ShouldBeNil)
					})

					Convey("invalid", func() {
						req := &pb.SearchBuildsRequest{
							Predicate: &pb.BuildPredicate{
								Builder: &pb.BuilderID{
									Project: "project",
									Bucket:  "bucket!",
								},
							},
						}
						err := validateSearch(req)
						So(err, ShouldErrLike, "bucket must match")
					})

					Convey("v1", func() {
						req := &pb.SearchBuildsRequest{
							Predicate: &pb.BuildPredicate{
								Builder: &pb.BuilderID{
									Project: "project",
									Bucket:  "luci.project.bucket",
								},
							},
						}
						err := validateSearch(req)
						So(err, ShouldErrLike, "invalid use of v1 builder.bucket in v2 API")
					})

					Convey("valid", func() {
						req := &pb.SearchBuildsRequest{
							Predicate: &pb.BuildPredicate{
								Builder: &pb.BuilderID{
									Project: "project",
									Bucket:  "bucket",
								},
							},
						}
						err := validateSearch(req)
						So(err, ShouldBeNil)
					})
				})

				Convey("builder", func() {
					Convey("empty", func() {
						req := &pb.SearchBuildsRequest{
							Predicate: &pb.BuildPredicate{
								Builder: &pb.BuilderID{
									Project: "project",
								},
							},
						}
						err := validateSearch(req)
						So(err, ShouldBeNil)
					})

					Convey("invalid", func() {
						req := &pb.SearchBuildsRequest{
							Predicate: &pb.BuildPredicate{
								Builder: &pb.BuilderID{
									Project: "project",
									Builder: "builder!",
								},
							},
						}
						err := validateSearch(req)
						So(err, ShouldErrLike, "builder must match")
					})

					Convey("valid", func() {
						req := &pb.SearchBuildsRequest{
							Predicate: &pb.BuildPredicate{
								Builder: &pb.BuilderID{
									Project: "project",
									Builder: "builder",
								},
							},
						}
						err := validateSearch(req)
						So(err, ShouldBeNil)
					})
				})

				Convey("gerrit change", func() {
					Convey("host", func() {
						req := &pb.SearchBuildsRequest{
							Predicate: &pb.BuildPredicate{
								GerritChanges: []*pb.GerritChange{
									{},
								},
							},
						}
						err := validateSearch(req)
						So(err, ShouldErrLike, "host is required")
					})

					Convey("change", func() {
						req := &pb.SearchBuildsRequest{
							Predicate: &pb.BuildPredicate{
								GerritChanges: []*pb.GerritChange{
									{
										Host: "host",
									},
								},
							},
						}
						err := validateSearch(req)
						So(err, ShouldErrLike, "change is required")
					})

					Convey("patchset", func() {
						req := &pb.SearchBuildsRequest{
							Predicate: &pb.BuildPredicate{
								GerritChanges: []*pb.GerritChange{
									{
										Host:   "host",
										Change: 1,
									},
								},
							},
						}
						err := validateSearch(req)
						So(err, ShouldErrLike, "patchset is required")
					})

					Convey("valid", func() {
						req := &pb.SearchBuildsRequest{
							Predicate: &pb.BuildPredicate{
								GerritChanges: []*pb.GerritChange{
									{
										Host:     "host",
										Change:   1,
										Patchset: 1,
									},
								},
							},
						}
						err := validateSearch(req)
						So(err, ShouldBeNil)
					})
				})

				Convey("output gitiles commit", func() {
					Convey("host", func() {
						req := &pb.SearchBuildsRequest{
							Predicate: &pb.BuildPredicate{
								OutputGitilesCommit: &pb.GitilesCommit{},
							},
						}
						err := validateSearch(req)
						So(err, ShouldErrLike, "host is required")
					})

					Convey("project", func() {
						req := &pb.SearchBuildsRequest{
							Predicate: &pb.BuildPredicate{
								OutputGitilesCommit: &pb.GitilesCommit{
									Host: "host",
								},
							},
						}
						err := validateSearch(req)
						So(err, ShouldErrLike, "project is required")
					})

					Convey("id", func() {
						req := &pb.SearchBuildsRequest{
							Predicate: &pb.BuildPredicate{
								OutputGitilesCommit: &pb.GitilesCommit{
									Host:    "host",
									Project: "project",
									Id:      "id",
								},
							},
						}
						err := validateSearch(req)
						So(err, ShouldErrLike, "id must match")
					})

					Convey("ref", func() {
						req := &pb.SearchBuildsRequest{
							Predicate: &pb.BuildPredicate{
								OutputGitilesCommit: &pb.GitilesCommit{
									Host:    "host",
									Project: "project",
									Ref:     "ref",
								},
							},
						}
						err := validateSearch(req)
						So(err, ShouldErrLike, "ref must match")
					})

					Convey("mutual exclusion", func() {
						Convey("ref", func() {
							req := &pb.SearchBuildsRequest{
								Predicate: &pb.BuildPredicate{
									OutputGitilesCommit: &pb.GitilesCommit{
										Host:    "host",
										Project: "project",
										Id:      "id",
										Ref:     "ref",
									},
								},
							}
							err := validateSearch(req)
							So(err, ShouldErrLike, "id is mutually exclusive with (ref and position)")
						})

						Convey("position", func() {
							req := &pb.SearchBuildsRequest{
								Predicate: &pb.BuildPredicate{
									OutputGitilesCommit: &pb.GitilesCommit{
										Host:     "host",
										Project:  "project",
										Id:       "id",
										Position: 1,
									},
								},
							}
							err := validateSearch(req)
							So(err, ShouldErrLike, "id is mutually exclusive with (ref and position)")
						})

						Convey("neither", func() {
							req := &pb.SearchBuildsRequest{
								Predicate: &pb.BuildPredicate{
									OutputGitilesCommit: &pb.GitilesCommit{
										Host:    "host",
										Project: "project",
									},
								},
							}
							err := validateSearch(req)
							So(err, ShouldErrLike, "one of")
						})
					})

					Convey("valid", func() {
						Convey("id", func() {
							req := &pb.SearchBuildsRequest{
								Predicate: &pb.BuildPredicate{
									OutputGitilesCommit: &pb.GitilesCommit{
										Host:    "host",
										Project: "project",
										Id:      "1234567890123456789012345678901234567890",
									},
								},
							}
							err := validateSearch(req)
							So(err, ShouldBeNil)
						})

						Convey("ref", func() {
							req := &pb.SearchBuildsRequest{
								Predicate: &pb.BuildPredicate{
									OutputGitilesCommit: &pb.GitilesCommit{
										Host:     "host",
										Project:  "project",
										Ref:      "refs/ref",
										Position: 1,
									},
								},
							}
							err := validateSearch(req)
							So(err, ShouldBeNil)
						})
					})
				})
			})

			Convey("mutual exclusion", func() {
				req := &pb.SearchBuildsRequest{
					Predicate: &pb.BuildPredicate{
						Build:      &pb.BuildRange{},
						CreateTime: &pb.TimeRange{},
					},
				}
				err := validateSearch(req)
				So(err, ShouldErrLike, "mutually exclusive")
			})
		})
	})

	Convey("CancelBuild", t, func() {
		srv := &Builds{}
		ctx := memory.Use(context.Background())
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)

		Convey("id", func() {
			Convey("not found", func() {
				req := &pb.CancelBuildRequest{
					Id:              1,
					SummaryMarkdown: "summary",
				}
				rsp, err := srv.CancelBuild(ctx, req)
				So(err, ShouldErrLike, "not found")
				So(rsp, ShouldBeNil)
			})

			Convey("permission denied", func() {
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity: identity.Identity("user:user"),
				})
				So(datastore.Put(ctx, &model.Bucket{
					ID: "bucket",
					Parent: datastore.KeyForObj(ctx, &model.Project{
						ID: "project",
					}),
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
				}), ShouldBeNil)
				req := &pb.CancelBuildRequest{
					Id:              1,
					SummaryMarkdown: "summary",
				}
				rsp, err := srv.CancelBuild(ctx, req)
				So(err, ShouldErrLike, "does not have permission")
				So(rsp, ShouldBeNil)
			})

			Convey("found", func() {
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity: identity.Identity("user:user"),
				})
				So(datastore.Put(ctx, &model.Bucket{
					ID: "bucket",
					Parent: datastore.KeyForObj(ctx, &model.Project{
						ID: "project",
					}),
					Proto: pb.Bucket{
						Acls: []*pb.Acl{
							{
								Identity: "user:user",
								Role:     pb.Acl_WRITER,
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
				}), ShouldBeNil)
				req := &pb.CancelBuildRequest{
					Id:              1,
					SummaryMarkdown: "summary",
				}
				rsp, err := srv.CancelBuild(ctx, req)
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
}
