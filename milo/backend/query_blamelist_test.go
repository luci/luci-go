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

package backend

import (
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"

	. "github.com/smartystreets/goconvey/convey"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/appengine/gaetesting"
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
	gitpb "go.chromium.org/luci/common/proto/git"
	milopb "go.chromium.org/luci/milo/api/service/v1"
	"go.chromium.org/luci/milo/common"
	"go.chromium.org/luci/milo/common/model"
	"go.chromium.org/luci/milo/git"
)

func TestPrepareQueryBlamelistRequest(t *testing.T) {
	t.Parallel()
	Convey(`TestPrepareQueryBlamelistRequest`, t, func() {
		Convey(`extract commit ID correctly`, func() {
			Convey(`when there's no page token`, func() {
				startCommitID, err := prepareQueryBlamelistRequest(&milopb.QueryBlamelistRequest{
					GitilesCommit: &buildbucketpb.GitilesCommit{
						Host:    "host",
						Project: "project/src",
						Id:      "commit-id",
					},
					Builder: &buildbucketpb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
				})
				So(err, ShouldBeNil)
				So(startCommitID, ShouldEqual, "commit-id")
			})

			Convey(`when there's a page token`, func() {
				commitID, err := prepareQueryBlamelistRequest(&milopb.QueryBlamelistRequest{
					GitilesCommit: &buildbucketpb.GitilesCommit{
						Host:    "host",
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
				So(commitID, ShouldEqual, "commit-id-1")

				pageToken, err := serializeQueryBlamelistPageToken(&milopb.QueryBlamelistPageToken{
					NextCommitId: "commit-id-2",
				})
				So(err, ShouldBeNil)

				nextCommitID, err := prepareQueryBlamelistRequest(&milopb.QueryBlamelistRequest{
					GitilesCommit: &buildbucketpb.GitilesCommit{
						Host:    "host",
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
				So(nextCommitID, ShouldEqual, "commit-id-2")
			})
		})

		Convey(`reject the page token when the page token is invalid`, func() {
			_, err := prepareQueryBlamelistRequest(&milopb.QueryBlamelistRequest{
				GitilesCommit: &buildbucketpb.GitilesCommit{
					Host:    "host",
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
	})
}

func TestQueryBlamelist(t *testing.T) {
	t.Parallel()
	Convey(`TestQueryBlamelist`, t, func() {
		c := gaetesting.TestingContextWithAppID("luci-milo-dev")
		datastore.GetTestable(c).Consistent(true)
		srv := &MiloInternalService{}
		ctrl := gomock.NewController(t)
		gitMock := git.NewMockClient(ctrl)
		c = git.Use(c, gitMock)

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
			&gitpb.Commit{Id: "commit8"},
			&gitpb.Commit{Id: "commit7"},
			&gitpb.Commit{Id: "commit6"},
			&gitpb.Commit{Id: "commit5"},
			&gitpb.Commit{Id: "commit4"},
			&gitpb.Commit{Id: "commit3"},
			&gitpb.Commit{Id: "commit2"},
			&gitpb.Commit{Id: "commit1"},
		}

		createFakeBuild := func(builder *buildbucketpb.BuilderID, buildNum int, commitID string) *model.BuildSummary {
			builderID := common.LegacyBuilderIDString(builder)
			buildID := fmt.Sprintf("%s/%d", builderID, buildNum)
			return &model.BuildSummary{
				BuildKey:  datastore.MakeKey(c, "build", buildID),
				ProjectID: builder.Project,
				BuilderID: builderID,
				BuildID:   buildID,
				BuildSet: []string{protoutil.GitilesBuildSet(&buildbucketpb.GitilesCommit{
					Host:    "fake_gitiles_host",
					Project: "fake_gitiles_project",
					Id:      commitID,
				})},
			}
		}

		builds := []*model.BuildSummary{
			createFakeBuild(builder1, 1, "commit8"),
			createFakeBuild(builder2, 1, "commit7"),
			createFakeBuild(builder1, 2, "commit5"),
			createFakeBuild(builder1, 3, "commit3"),
		}

		err := datastore.Put(c, builds)
		So(err, ShouldBeNil)

		Convey(`coerce page_size`, func() {
			Convey(`to 1000 if it's greater than 1000`, func() {
				req := &milopb.QueryBlamelistRequest{
					GitilesCommit: &buildbucketpb.GitilesCommit{
						Host:    "fake_gitiles_host",
						Project: "fake_gitiles_project",
						Id:      "commit1",
					},
					Builder:  builder1,
					PageSize: 2000,
				}
				gitMock.
					EXPECT().
					Log(c, req.GitilesCommit.Host, req.GitilesCommit.Project, req.GitilesCommit.Id, &git.LogOptions{Limit: 1001, WithFiles: true}).
					Return(commits, nil)

				_, err := srv.QueryBlamelist(c, req)
				So(err, ShouldBeNil)
			})

			Convey(`to 100 if it's not set`, func() {
				req := &milopb.QueryBlamelistRequest{
					GitilesCommit: &buildbucketpb.GitilesCommit{
						Host:    "fake_gitiles_host",
						Project: "fake_gitiles_project",
						Id:      "commit1",
					},
					Builder: builder1,
				}
				gitMock.
					EXPECT().
					Log(c, req.GitilesCommit.Host, req.GitilesCommit.Project, req.GitilesCommit.Id, &git.LogOptions{Limit: 101, WithFiles: true}).
					Return(commits, nil)

				_, err := srv.QueryBlamelist(c, req)
				So(err, ShouldBeNil)
			})
		})

		Convey(`get all the commits in the blamelist`, func() {
			Convey(`in one page`, func() {
				req := &milopb.QueryBlamelistRequest{
					GitilesCommit: &buildbucketpb.GitilesCommit{
						Host:    "fake_gitiles_host",
						Project: "fake_gitiles_project",
						Id:      "commit1",
					},
					Builder: builder1,
				}
				gitMock.
					EXPECT().
					Log(c, req.GitilesCommit.Host, req.GitilesCommit.Project, req.GitilesCommit.Id, &git.LogOptions{Limit: 101, WithFiles: true}).
					Return(commits, nil)

				res, err := srv.QueryBlamelist(c, req)
				So(err, ShouldBeNil)
				So(res.Commits, ShouldHaveLength, 3)
				So(res.Commits[0].Id, ShouldEqual, "commit8")
				So(res.Commits[1].Id, ShouldEqual, "commit7")
				So(res.Commits[2].Id, ShouldEqual, "commit6")
			})
			Convey(`in multiple pages`, func() {
				// Query the first page.
				req := &milopb.QueryBlamelistRequest{
					GitilesCommit: &buildbucketpb.GitilesCommit{
						Host:    "fake_gitiles_host",
						Project: "fake_gitiles_project",
						Id:      "commit1",
					},
					Builder:  builder1,
					PageSize: 2,
				}

				gitMock.
					EXPECT().
					Log(c, req.GitilesCommit.Host, req.GitilesCommit.Project, req.GitilesCommit.Id, &git.LogOptions{Limit: 3, WithFiles: true}).
					Return(commits[0:3], nil)

				res, err := srv.QueryBlamelist(c, req)
				So(err, ShouldBeNil)
				So(res.Commits, ShouldHaveLength, 2)
				So(res.Commits[0].Id, ShouldEqual, "commit8")
				So(res.Commits[1].Id, ShouldEqual, "commit7")
				So(res.NextPageToken, ShouldNotBeZeroValue)

				// Query the second page.
				req = &milopb.QueryBlamelistRequest{
					GitilesCommit: &buildbucketpb.GitilesCommit{
						Host:    "fake_gitiles_host",
						Project: "fake_gitiles_project",
						Id:      "commit1",
					},
					Builder:   builder1,
					PageSize:  2,
					PageToken: res.NextPageToken,
				}

				gitMock.
					EXPECT().
					Log(c, req.GitilesCommit.Host, req.GitilesCommit.Project, "commit6", &git.LogOptions{Limit: 3, WithFiles: true}).
					Return(commits[2:5], nil)

				res, err = srv.QueryBlamelist(c, req)
				So(err, ShouldBeNil)
				So(res.Commits, ShouldHaveLength, 1)
				So(res.Commits[0].Id, ShouldEqual, "commit6")
				So(res.NextPageToken, ShouldBeZeroValue)
			})
		})
	})
}
