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

	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/auth/identity"
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
	gitpb "go.chromium.org/luci/common/proto/git"
	"go.chromium.org/luci/common/proto/gitiles"
	"go.chromium.org/luci/common/proto/gitiles/mock_gitiles"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
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
	ftt.Run(`TestPrepareQueryBlamelistRequest`, t, func(t *ftt.Test) {
		t.Run(`extract commit ID correctly`, func(t *ftt.Test) {
			t.Run(`when there's no page token`, func(t *ftt.Test) {
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
				assert.Loosely(t, err, should.BeNil)
				// commit ID should take priority.
				assert.Loosely(t, startRev, should.Equal("commit-id"))
			})

			t.Run(`when there's no page token or commit ID`, func(t *ftt.Test) {
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
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, startRev, should.Equal("commit-ref"))
			})

			t.Run(`when there's a page token`, func(t *ftt.Test) {
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
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, startRev, should.Equal("commit-id-1"))

				pageToken, err := serializeQueryBlamelistPageToken(&milopb.QueryBlamelistPageToken{
					NextCommitId: "commit-id-2",
				})
				assert.Loosely(t, err, should.BeNil)

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
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, nextCommitRev, should.Equal("commit-id-2"))
			})
		})

		t.Run(`reject the page token when the page token is invalid`, func(t *ftt.Test) {
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
			assert.Loosely(t, err, should.NotBeNil)
		})

		t.Run("no builder", func(t *ftt.Test) {
			_, err := prepareQueryBlamelistRequest(&milopb.QueryBlamelistRequest{
				GitilesCommit: &buildbucketpb.GitilesCommit{
					Host:    "chromium.googlesource.com",
					Project: "project/src",
					Id:      "commit-id",
					Ref:     "commit-ref",
				},
			})
			assert.Loosely(t, err, should.ErrLike("builder: project must match"))
		})

		t.Run("invalid builder", func(t *ftt.Test) {
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
			assert.Loosely(t, err, should.ErrLike("builder: bucket must match"))
		})

		t.Run(`invalid gitiles host`, func(t *ftt.Test) {
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
			assert.Loosely(t, err, should.ErrLike("gitiles_commit.host must be a subdomain of .googlesource.com"))
		})

	})
}

func TestQueryBlamelist(t *testing.T) {
	t.Parallel()
	ftt.Run(`TestQueryBlamelist`, t, func(t *ftt.Test) {
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
		assert.Loosely(t, err, should.BeNil)

		err = datastore.Put(c, &projectconfig.Project{
			ID:      "fake_project",
			ACL:     projectconfig.ACL{Identities: []identity.Identity{"user"}},
			LogoURL: "https://logo.com",
		})
		assert.Loosely(t, err, should.BeNil)

		t.Run(`reject users with no access`, func(t *ftt.Test) {
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
			assert.Loosely(t, err, should.NotBeNil)
		})

		t.Run(`coerce page_size`, func(t *ftt.Test) {
			t.Run(`to 1000 if it's greater than 1000`, func(t *ftt.Test) {
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
				assert.Loosely(t, err, should.BeNil)
			})

			t.Run(`to 100 if it's not set`, func(t *ftt.Test) {
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
				assert.Loosely(t, err, should.BeNil)
			})
		})

		t.Run(`get all the commits in the blamelist`, func(t *ftt.Test) {
			t.Run(`in one page`, func(t *ftt.Test) {
				t.Run(`when we found the previous build`, func(t *ftt.Test) {
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
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, res.Commits, should.HaveLength(3))
					assert.Loosely(t, res.Commits[0].Id, should.Equal("commit8"))
					assert.Loosely(t, res.Commits[1].Id, should.Equal("commit7"))
					assert.Loosely(t, res.Commits[2].Id, should.Equal("commit6"))
					assert.Loosely(t, res.PrecedingCommit.Id, should.Equal("commit5"))
				})

				t.Run(`when there's no previous build`, func(t *ftt.Test) {
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
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, res.Commits, should.HaveLength(3))
					assert.Loosely(t, res.Commits[0].Id, should.Equal("commit3"))
					assert.Loosely(t, res.Commits[1].Id, should.Equal("commit2"))
					assert.Loosely(t, res.Commits[2].Id, should.Equal("commit1"))
					assert.Loosely(t, res.PrecedingCommit.String(), should.Equal("<nil>"))
				})
			})

			t.Run(`in multiple pages`, func(t *ftt.Test) {
				t.Run(`when we found the previous build`, func(t *ftt.Test) {
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
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, res.Commits, should.HaveLength(2))
					assert.Loosely(t, res.Commits[0].Id, should.Equal("commit8"))
					assert.Loosely(t, res.Commits[1].Id, should.Equal("commit7"))
					assert.Loosely(t, res.NextPageToken, should.NotEqual(""))
					assert.Loosely(t, res.PrecedingCommit.Id, should.Equal("commit6"))

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
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, res.Commits, should.HaveLength(1))
					assert.Loosely(t, res.Commits[0].Id, should.Equal("commit6"))
					assert.Loosely(t, res.NextPageToken, should.Equal(""))
					assert.Loosely(t, res.PrecedingCommit.Id, should.Equal("commit5"))
				})

				t.Run(`when there's no previous build`, func(t *ftt.Test) {
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
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, res.Commits, should.HaveLength(2))
					assert.Loosely(t, res.Commits[0].Id, should.Equal("commit3"))
					assert.Loosely(t, res.Commits[1].Id, should.Equal("commit2"))
					assert.Loosely(t, res.NextPageToken, should.NotEqual(""))
					assert.Loosely(t, res.PrecedingCommit.Id, should.Equal("commit1"))

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
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, res.Commits, should.HaveLength(1))
					assert.Loosely(t, res.Commits[0].Id, should.Equal("commit1"))
					assert.Loosely(t, res.NextPageToken, should.Equal(""))
					assert.Loosely(t, res.PrecedingCommit.String(), should.Equal("<nil>"))
				})
			})
		})

		t.Run(`get blamelist of other projects`, func(t *ftt.Test) {
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
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res.Commits, should.HaveLength(2))
			assert.Loosely(t, res.Commits[0].Id, should.Equal("commit3"))
			assert.Loosely(t, res.Commits[1].Id, should.Equal("commit2"))
			assert.Loosely(t, res.PrecedingCommit.Id, should.Equal("commit1"))
		})
	})
}
