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
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"

	. "github.com/smartystreets/goconvey/convey"
	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/auth/identity"
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
	gitpb "go.chromium.org/luci/common/proto/git"
	"go.chromium.org/luci/common/proto/gitiles"
	"go.chromium.org/luci/common/proto/gitiles/mock_gitiles"
	. "go.chromium.org/luci/common/testing/assertions"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/milo/internal/model"
	"go.chromium.org/luci/milo/internal/projectconfig"
	"go.chromium.org/luci/milo/internal/utils"
	milopb "go.chromium.org/luci/milo/proto/v1"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
)

func TestPrepareQueryBlamelistRequest(t *testing.T) {
	t.Parallel()
	Convey(`TestPrepareQueryBlamelistRequest`, t, func() {
		Convey(`extract commit ID correctly`, func() {
			Convey(`when there's no page token`, func() {
				startRev, err := prepareQueryBlamelistRequest(&milopb.QueryBlamelistRequest{
					GitilesCommit: &buildbucketpb.GitilesCommit{
						Host:    "chromium.googlesource.com",
						Project: "project/src",
						Id:      "commit-id",
						Ref:     "commit-ref",
					},
					Builder: &buildbucketpb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
				})
				So(err, ShouldBeNil)
				// commit ID should take priority.
				So(startRev, ShouldEqual, "commit-id")
			})

			Convey(`when there's no page token or commit ID`, func() {
				startRev, err := prepareQueryBlamelistRequest(&milopb.QueryBlamelistRequest{
					GitilesCommit: &buildbucketpb.GitilesCommit{
						Host:    "chromium.googlesource.com",
						Project: "project/src",
						Ref:     "commit-ref",
					},
					Builder: &buildbucketpb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
				})
				So(err, ShouldBeNil)
				So(startRev, ShouldEqual, "commit-ref")
			})

			Convey(`when there's a page token`, func() {
				startRev, err := prepareQueryBlamelistRequest(&milopb.QueryBlamelistRequest{
					GitilesCommit: &buildbucketpb.GitilesCommit{
						Host:    "chromium.googlesource.com",
						Project: "project/src",
						Id:      "commit-id-1",
					},
					Builder: &buildbucketpb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
				})
				So(err, ShouldBeNil)
				So(startRev, ShouldEqual, "commit-id-1")

				pageToken, err := serializeQueryBlamelistPageToken(&milopb.QueryBlamelistPageToken{
					NextCommitId: "commit-id-2",
				})
				So(err, ShouldBeNil)

				nextCommitRev, err := prepareQueryBlamelistRequest(&milopb.QueryBlamelistRequest{
					GitilesCommit: &buildbucketpb.GitilesCommit{
						Host:    "chromium.googlesource.com",
						Project: "project/src",
						Id:      "commit-id-1",
					},
					Builder: &buildbucketpb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					PageToken: pageToken,
				})
				So(err, ShouldBeNil)
				So(nextCommitRev, ShouldEqual, "commit-id-2")
			})
		})

		Convey(`reject the page token when the page token is invalid`, func() {
			_, err := prepareQueryBlamelistRequest(&milopb.QueryBlamelistRequest{
				GitilesCommit: &buildbucketpb.GitilesCommit{
					Host:    "chromium.googlesource.com",
					Project: "project/src",
					Id:      "commit-id-1",
				},
				Builder: &buildbucketpb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				PageToken: "abc",
			})
			So(err, ShouldNotBeNil)
		})

		Convey("no builder", func() {
			_, err := prepareQueryBlamelistRequest(&milopb.QueryBlamelistRequest{
				GitilesCommit: &buildbucketpb.GitilesCommit{
					Host:    "chromium.googlesource.com",
					Project: "project/src",
					Id:      "commit-id",
					Ref:     "commit-ref",
				},
			})
			So(err, ShouldErrLike, "builder: project must match")
		})

		Convey("invalid builder", func() {
			_, err := prepareQueryBlamelistRequest(&milopb.QueryBlamelistRequest{
				GitilesCommit: &buildbucketpb.GitilesCommit{
					Host:    "chromium.googlesource.com",
					Project: "project/src",
					Id:      "commit-id",
					Ref:     "commit-ref",
				},
				Builder: &buildbucketpb.BuilderID{
					Project: "fake_proj",
					Bucket:  "fake[]ucket",
					Builder: "fake_/uilder1",
				},
			})
			So(err, ShouldErrLike, "builder: bucket must match")
		})

		Convey(`invalid gitiles host`, func() {
			_, err := prepareQueryBlamelistRequest(&milopb.QueryBlamelistRequest{
				GitilesCommit: &buildbucketpb.GitilesCommit{
					Host:    "invalid.host",
					Project: "project/src",
					Id:      "commit-id",
					Ref:     "commit-ref",
				},
				Builder: &buildbucketpb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
			})
			So(err, ShouldErrLike, "gitiles_commit.host must be a subdomain of .googlesource.com")
		})

	})
}

func TestQueryBlamelist(t *testing.T) {
	t.Parallel()
	Convey(`TestQueryBlamelist`, t, func() {
		c := gaetesting.TestingContextWithAppID("luci-milo-dev")
		datastore.GetTestable(c).Consistent(true)
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		gitMock := mock_gitiles.NewMockGitilesClient(ctrl)
		srv := &MiloInternalService{
			GetGitilesClient: func(c context.Context, host string, as auth.RPCAuthorityKind) (gitiles.GitilesClient, error) {
				return gitMock, nil
			},
		}
		c = auth.WithState(c, &authtest.FakeState{Identity: "user"})

		builder1 := &buildbucketpb.BuilderID{
			Project: "fake_project",
			Bucket:  "fake_bucket",
			Builder: "fake_builder1",
		}
		builder2 := &buildbucketpb.BuilderID{
			Project: "fake_project",
			Bucket:  "fake_bucket",
			Builder: "fake_builder2",
		}

		commits := []*gitpb.Commit{
			{Id: "commit8"},
			{Id: "commit7"},
			{Id: "commit6"},
			{Id: "commit5"},
			{Id: "commit4"},
			{Id: "commit3"},
			{Id: "commit2"},
			{Id: "commit1"},
		}

		createFakeBuild := func(builder *buildbucketpb.BuilderID, buildNum int, commitID string, additionalBlamelistPins ...string) *model.BuildSummary {
			builderID := utils.LegacyBuilderIDString(builder)
			buildID := fmt.Sprintf("%s/%d", builderID, buildNum)
			buildSet := protoutil.GitilesBuildSet(&buildbucketpb.GitilesCommit{
				Host:    "fake_host.googlesource.com",
				Project: "fake_gitiles_project",
				Id:      commitID,
			})
			blamelistPins := append(additionalBlamelistPins, buildSet)
			return &model.BuildSummary{
				BuildKey:      datastore.MakeKey(c, "build", buildID),
				ProjectID:     builder.Project,
				BuilderID:     builderID,
				BuildID:       buildID,
				BuildSet:      []string{buildSet},
				BlamelistPins: blamelistPins,
			}
		}

		builds := []*model.BuildSummary{
			createFakeBuild(builder1, 1, "commit8", protoutil.GitilesBuildSet(&buildbucketpb.GitilesCommit{
				Host:    "fake_host.googlesource.com",
				Project: "other_fake_gitiles_project",
				Id:      "commit3",
			})),
			createFakeBuild(builder2, 1, "commit7"),
			createFakeBuild(builder1, 2, "commit5", protoutil.GitilesBuildSet(&buildbucketpb.GitilesCommit{
				Host:    "fake_host.googlesource.com",
				Project: "other_fake_gitiles_project",
				Id:      "commit1",
			})),
			createFakeBuild(builder1, 3, "commit3"),
		}

		err := datastore.Put(c, builds)
		So(err, ShouldBeNil)

		err = datastore.Put(c, &projectconfig.Project{
			ID:      "fake_project",
			ACL:     projectconfig.ACL{Identities: []identity.Identity{"user"}},
			LogoURL: "https://logo.com",
		})
		So(err, ShouldBeNil)

		Convey(`reject users with no access`, func() {
			req := &milopb.QueryBlamelistRequest{
				GitilesCommit: &buildbucketpb.GitilesCommit{
					Host:    "fake_host.googlesource.com",
					Project: "fake_gitiles_project",
					Id:      "commit1",
				},
				Builder: &buildbucketpb.BuilderID{
					Project: "secret_fake_project",
					Bucket:  "secret_fake_bucket",
					Builder: "secret_fake_builder",
				},
				PageSize: 2000,
			}
			_, err := srv.QueryBlamelist(c, req)
			So(err, ShouldNotBeNil)
		})

		Convey(`coerce page_size`, func() {
			Convey(`to 1000 if it's greater than 1000`, func() {
				req := &milopb.QueryBlamelistRequest{
					GitilesCommit: &buildbucketpb.GitilesCommit{
						Host:    "fake_host.googlesource.com",
						Project: "fake_gitiles_project",
						Id:      "commit1",
					},
					Builder:  builder1,
					PageSize: 2000,
				}
				gitMock.
					EXPECT().
					Log(gomock.Any(), &gitiles.LogRequest{
						Project:    "fake_gitiles_project",
						Committish: "commit1",
						PageSize:   1001,
						TreeDiff:   true,
					}).
					Return(&gitiles.LogResponse{
						Log: commits,
					}, nil)

				_, err := srv.QueryBlamelist(c, req)
				So(err, ShouldBeNil)
			})

			Convey(`to 100 if it's not set`, func() {
				req := &milopb.QueryBlamelistRequest{
					GitilesCommit: &buildbucketpb.GitilesCommit{
						Host:    "fake_host.googlesource.com",
						Project: "fake_gitiles_project",
						Id:      "commit8",
					},
					Builder: builder1,
				}
				gitMock.
					EXPECT().
					Log(gomock.Any(), &gitiles.LogRequest{
						Project:    req.GitilesCommit.Project,
						Committish: req.GitilesCommit.Id,
						PageSize:   101,
						TreeDiff:   true,
					}).
					Return(&gitiles.LogResponse{
						Log: commits,
					}, nil)

				_, err := srv.QueryBlamelist(c, req)
				So(err, ShouldBeNil)
			})
		})

		Convey(`get all the commits in the blamelist`, func() {
			Convey(`in one page`, func() {
				Convey(`when we found the previous build`, func() {
					req := &milopb.QueryBlamelistRequest{
						GitilesCommit: &buildbucketpb.GitilesCommit{
							Host:    "fake_host.googlesource.com",
							Project: "fake_gitiles_project",
							Id:      "commit8",
						},
						Builder: builder1,
					}
					gitMock.
						EXPECT().
						Log(gomock.Any(), &gitiles.LogRequest{
							Project:    req.GitilesCommit.Project,
							Committish: req.GitilesCommit.Id,
							PageSize:   101,
							TreeDiff:   true,
						}).
						Return(&gitiles.LogResponse{
							Log: commits,
						}, nil)

					res, err := srv.QueryBlamelist(c, req)
					So(err, ShouldBeNil)
					So(res.Commits, ShouldHaveLength, 3)
					So(res.Commits[0].Id, ShouldEqual, "commit8")
					So(res.Commits[1].Id, ShouldEqual, "commit7")
					So(res.Commits[2].Id, ShouldEqual, "commit6")
					So(res.PrecedingCommit.Id, ShouldEqual, "commit5")
				})

				Convey(`when there's no previous build`, func() {
					req := &milopb.QueryBlamelistRequest{
						GitilesCommit: &buildbucketpb.GitilesCommit{
							Host:    "fake_host.googlesource.com",
							Project: "fake_gitiles_project",
							Id:      "commit3",
						},
						Builder: builder1,
					}
					gitMock.
						EXPECT().
						Log(gomock.Any(), &gitiles.LogRequest{
							Project:    req.GitilesCommit.Project,
							Committish: req.GitilesCommit.Id,
							PageSize:   101,
							TreeDiff:   true,
						}).
						Return(&gitiles.LogResponse{
							Log: commits[5:],
						}, nil)

					res, err := srv.QueryBlamelist(c, req)
					So(err, ShouldBeNil)
					So(res.Commits, ShouldHaveLength, 3)
					So(res.Commits[0].Id, ShouldEqual, "commit3")
					So(res.Commits[1].Id, ShouldEqual, "commit2")
					So(res.Commits[2].Id, ShouldEqual, "commit1")
					So(res.PrecedingCommit, ShouldBeZeroValue)
				})
			})

			Convey(`in multiple pages`, func() {
				Convey(`when we found the previous build`, func() {
					// Query the first page.
					req := &milopb.QueryBlamelistRequest{
						GitilesCommit: &buildbucketpb.GitilesCommit{
							Host:    "fake_host.googlesource.com",
							Project: "fake_gitiles_project",
							Id:      "commit8",
						},
						Builder:  builder1,
						PageSize: 2,
					}

					gitMock.
						EXPECT().
						Log(gomock.Any(), &gitiles.LogRequest{
							Project:    req.GitilesCommit.Project,
							Committish: req.GitilesCommit.Id,
							PageSize:   3,
							TreeDiff:   true,
						}).
						Return(&gitiles.LogResponse{
							Log: commits[0:3],
						}, nil)

					res, err := srv.QueryBlamelist(c, req)
					So(err, ShouldBeNil)
					So(res.Commits, ShouldHaveLength, 2)
					So(res.Commits[0].Id, ShouldEqual, "commit8")
					So(res.Commits[1].Id, ShouldEqual, "commit7")
					So(res.NextPageToken, ShouldNotBeZeroValue)
					So(res.PrecedingCommit.Id, ShouldEqual, "commit6")

					// Query the second page.
					req = &milopb.QueryBlamelistRequest{
						GitilesCommit: &buildbucketpb.GitilesCommit{
							Host:    "fake_host.googlesource.com",
							Project: "fake_gitiles_project",
							Id:      "commit8",
						},
						Builder:   builder1,
						PageSize:  2,
						PageToken: res.NextPageToken,
					}

					gitMock.
						EXPECT().
						Log(gomock.Any(), &gitiles.LogRequest{
							Project:    req.GitilesCommit.Project,
							Committish: "commit6",
							PageSize:   3,
							TreeDiff:   true,
						}).
						Return(&gitiles.LogResponse{
							Log: commits[2:5],
						}, nil)

					res, err = srv.QueryBlamelist(c, req)
					So(err, ShouldBeNil)
					So(res.Commits, ShouldHaveLength, 1)
					So(res.Commits[0].Id, ShouldEqual, "commit6")
					So(res.NextPageToken, ShouldBeZeroValue)
					So(res.PrecedingCommit.Id, ShouldEqual, "commit5")
				})

				Convey(`when there's no previous build`, func() {
					// Query the first page.
					req := &milopb.QueryBlamelistRequest{
						GitilesCommit: &buildbucketpb.GitilesCommit{
							Host:    "fake_host.googlesource.com",
							Project: "fake_gitiles_project",
							Id:      "commit3",
						},
						Builder:  builder1,
						PageSize: 2,
					}

					gitMock.
						EXPECT().
						Log(gomock.Any(), &gitiles.LogRequest{
							Project:    req.GitilesCommit.Project,
							Committish: req.GitilesCommit.Id,
							PageSize:   3,
							TreeDiff:   true,
						}).
						Return(&gitiles.LogResponse{
							Log: commits[5:8],
						}, nil)

					res, err := srv.QueryBlamelist(c, req)
					So(err, ShouldBeNil)
					So(res.Commits, ShouldHaveLength, 2)
					So(res.Commits[0].Id, ShouldEqual, "commit3")
					So(res.Commits[1].Id, ShouldEqual, "commit2")
					So(res.NextPageToken, ShouldNotBeZeroValue)
					So(res.PrecedingCommit.Id, ShouldEqual, "commit1")

					// Query the second page.
					req = &milopb.QueryBlamelistRequest{
						GitilesCommit: &buildbucketpb.GitilesCommit{
							Host:    "fake_host.googlesource.com",
							Project: "fake_gitiles_project",
							Id:      "commit3",
						},
						Builder:   builder1,
						PageSize:  2,
						PageToken: res.NextPageToken,
					}

					gitMock.
						EXPECT().
						Log(gomock.Any(), &gitiles.LogRequest{
							Project:    req.GitilesCommit.Project,
							Committish: "commit1",
							PageSize:   3,
							TreeDiff:   true,
						}).
						Return(&gitiles.LogResponse{
							Log: commits[7:],
						}, nil)

					res, err = srv.QueryBlamelist(c, req)
					So(err, ShouldBeNil)
					So(res.Commits, ShouldHaveLength, 1)
					So(res.Commits[0].Id, ShouldEqual, "commit1")
					So(res.NextPageToken, ShouldBeZeroValue)
					So(res.PrecedingCommit, ShouldBeZeroValue)
				})
			})
		})

		Convey(`get blamelist of other projects`, func() {
			req := &milopb.QueryBlamelistRequest{
				GitilesCommit: &buildbucketpb.GitilesCommit{
					Host:    "fake_host.googlesource.com",
					Project: "other_fake_gitiles_project",
					Id:      "commit3",
				},
				Builder: builder1,
			}
			gitMock.
				EXPECT().
				Log(gomock.Any(), &gitiles.LogRequest{
					Project:    req.GitilesCommit.Project,
					Committish: req.GitilesCommit.Id,
					PageSize:   101,
					TreeDiff:   true,
				}).
				Return(&gitiles.LogResponse{
					Log: commits[5:],
				}, nil)

			res, err := srv.QueryBlamelist(c, req)
			So(err, ShouldBeNil)
			So(res.Commits, ShouldHaveLength, 2)
			So(res.Commits[0].Id, ShouldEqual, "commit3")
			So(res.Commits[1].Id, ShouldEqual, "commit2")
			So(res.PrecedingCommit.Id, ShouldEqual, "commit1")
		})
	})
}
