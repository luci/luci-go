// Copyright 2023 The LUCI Authors.
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

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/buildbucket/bbperms"
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/secrets"

	"go.chromium.org/luci/milo/internal/model"
	"go.chromium.org/luci/milo/internal/model/milostatus"
	"go.chromium.org/luci/milo/internal/projectconfig"
	"go.chromium.org/luci/milo/internal/testutils"
	"go.chromium.org/luci/milo/internal/utils"
	projectconfigpb "go.chromium.org/luci/milo/proto/projectconfig"
	milopb "go.chromium.org/luci/milo/proto/v1"
)

var projects = []*projectconfig.Project{
	{
		ID:  "allowed-project",
		ACL: projectconfig.ACL{Identities: []identity.Identity{"user"}},
	},
	{
		ID:  "other-allowed-project",
		ACL: projectconfig.ACL{Identities: []identity.Identity{"user"}},
	},
	{
		ID:  "project-with-external-ref",
		ACL: projectconfig.ACL{Identities: []identity.Identity{"user"}},
	},
}

var consoleDefs = []*projectconfigpb.Console{
	{
		Realm: "allowed-project:@root",
		Id:    "con1",
		Builders: []*projectconfigpb.Builder{
			{
				Id: &buildbucketpb.BuilderID{
					Project: "allowed-project",
					Bucket:  "bucket",
					Builder: "builder",
				},
			},
			{
				Id: &buildbucketpb.BuilderID{
					Project: "other-allowed-project",
					Bucket:  "bucket",
					Builder: "builder",
				},
			},
			{
				Id: &buildbucketpb.BuilderID{
					Project: "forbidden-project",
					Bucket:  "bucket",
					Builder: "builder",
				},
			},
		},
	},
	{
		Realm: "allowed-project:@root",
		Id:    "con2",
		Builders: []*projectconfigpb.Builder{
			{
				Id: &buildbucketpb.BuilderID{
					Project: "allowed-project",
					Bucket:  "bucket",
					Builder: "builder",
				},
			},
			{
				Id: &buildbucketpb.BuilderID{
					Project: "allowed-project",
					Bucket:  "bucket",
					Builder: "new-builder",
				},
			},
		},
	},
	{
		Realm: "allowed-project:@root",
		Id:    "con3",
		Builders: []*projectconfigpb.Builder{
			{
				Id: &buildbucketpb.BuilderID{
					Project: "other-allowed-project",
					Bucket:  "bucket",
					Builder: "builder",
				},
			},
		},
	},
	{
		Realm: "other-allowed-project:@root",
		Id:    "con1",
		Builders: []*projectconfigpb.Builder{
			{
				Id: &buildbucketpb.BuilderID{
					Project: "allowed-project",
					Bucket:  "bucket",
					Builder: "builder",
				},
			},
		},
	},
	{
		Realm: "forbidden-project:@root",
		Id:    "con1",
		Builders: []*projectconfigpb.Builder{
			{
				Id: &buildbucketpb.BuilderID{
					Project: "allowed-project",
					Bucket:  "bucket",
					Builder: "builder",
				},
			},
		},
	},
	{
		Realm:           "project-with-external-ref:@root",
		Id:              "con1",
		ExternalProject: "allowed-project",
		ExternalId:      "con1",
	},
	{
		Realm:           "project-with-external-ref:@root",
		Id:              "con2",
		ExternalProject: "forbidden-project",
		ExternalId:      "con1",
	},
	{
		Realm: "project-with-external-ref:@root",
		Id:    "con3",
		Builders: []*projectconfigpb.Builder{
			{
				Id: &buildbucketpb.BuilderID{
					Project: "project-with-external-ref",
					Bucket:  "bucket",
					Builder: "builder",
				},
			},
		},
	},
}

var builderSummaries = []*model.BuilderSummary{
	{
		BuilderID:           "buildbucket/luci.allowed-project.bucket/builder",
		ProjectID:           "allowed-project",
		LastFinishedBuildID: "buildbucket/111111",
		LastFinishedStatus:  milostatus.InfraFailure,
	},
	{
		BuilderID:           "buildbucket/luci.other-allowed-project.bucket/builder",
		ProjectID:           "other-allowed-project",
		LastFinishedBuildID: "buildbucket/111112",
		LastFinishedStatus:  milostatus.Success,
	},
	{
		BuilderID:           "buildbucket/luci.forbidden-project.bucket/builder",
		ProjectID:           "forbidden-project",
		LastFinishedBuildID: "buildbucket/111113",
		LastFinishedStatus:  milostatus.Canceled,
	},
	{
		BuilderID:           "buildbucket/luci.project-with-external-ref.bucket/builder",
		ProjectID:           "project-with-external-ref",
		LastFinishedBuildID: "buildbucket/111114",
		LastFinishedStatus:  milostatus.Failure,
	},
}

var perms = []authtest.RealmPermission{
	{
		Realm:      "allowed-project:bucket",
		Permission: bbperms.BuildsList,
	},
	{
		Realm:      "other-allowed-project:bucket",
		Permission: bbperms.BuildsList,
	},
	{
		Realm:      "project-with-external-ref:bucket",
		Permission: bbperms.BuildsList,
	},
}

func TestQueryConsoleSnapshots(t *testing.T) {
	t.Parallel()
	ftt.Run(`TestQueryConsoleSnapshots`, t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		ctx = testutils.SetUpTestGlobalCache(ctx)
		ctx = secrets.GeneratePrimaryTinkAEADForTest(ctx)

		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)

		err := datastore.Put(ctx, projects)
		assert.Loosely(t, err, should.BeNil)

		// Transform & save console defs to datastore.
		consoles := make([]*projectconfig.Console, 0, len(consoleDefs))
		for _, conDef := range consoleDefs {
			proj, _ := realms.Split(conDef.Realm)
			conID := projectconfig.ConsoleID{
				Project: proj,
				ID:      conDef.Id,
			}
			console := conID.SetID(ctx, nil)
			console.Def = *conDef
			console.Builders = make([]string, 0, len(conDef.Builders))
			for _, builder := range conDef.Builders {
				legacyID := utils.LegacyBuilderIDString(builder.Id)
				console.Builders = append(console.Builders, legacyID)
			}
			consoles = append(consoles, console)
		}
		err = datastore.Put(ctx, consoles)
		assert.Loosely(t, err, should.BeNil)

		err = datastore.Put(ctx, builderSummaries)
		assert.Loosely(t, err, should.BeNil)

		srv := &MiloInternalService{}

		t.Run(`e2e`, func(t *ftt.Test) {
			ctx := auth.WithState(ctx, &authtest.FakeState{Identity: "user", IdentityPermissions: perms})

			res, err := srv.QueryConsoleSnapshots(ctx, &milopb.QueryConsoleSnapshotsRequest{
				Predicate: &milopb.ConsolePredicate{
					Project: "allowed-project",
				},
				PageSize: 2,
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res.Snapshots, should.Resemble([]*milopb.ConsoleSnapshot{
				{
					Console: &projectconfigpb.Console{
						Realm: "allowed-project:@root",
						Id:    "con1",
						Builders: []*projectconfigpb.Builder{
							{
								Id: &buildbucketpb.BuilderID{
									Project: "allowed-project",
									Bucket:  "bucket",
									Builder: "builder",
								},
							},
							{
								Id: &buildbucketpb.BuilderID{
									Project: "other-allowed-project",
									Bucket:  "bucket",
									Builder: "builder",
								},
							},
						},
					},
					BuilderSnapshots: []*milopb.BuilderSnapshot{
						{
							Builder: &buildbucketpb.BuilderID{
								Project: "allowed-project",
								Bucket:  "bucket",
								Builder: "builder",
							},
							Build: &buildbucketpb.Build{
								Id: 111111,
								Builder: &buildbucketpb.BuilderID{
									Project: "allowed-project",
									Bucket:  "bucket",
									Builder: "builder",
								},
								Status: buildbucketpb.Status_INFRA_FAILURE,
							},
						},
						{
							Builder: &buildbucketpb.BuilderID{
								Project: "other-allowed-project",
								Bucket:  "bucket",
								Builder: "builder",
							},
							Build: &buildbucketpb.Build{
								Id: 111112,
								Builder: &buildbucketpb.BuilderID{
									Project: "other-allowed-project",
									Bucket:  "bucket",
									Builder: "builder",
								},
								Status: buildbucketpb.Status_SUCCESS,
							},
						},
					},
				},
				{
					Console: &projectconfigpb.Console{
						Realm: "allowed-project:@root",
						Id:    "con2",
						Builders: []*projectconfigpb.Builder{
							{
								Id: &buildbucketpb.BuilderID{
									Project: "allowed-project",
									Bucket:  "bucket",
									Builder: "builder",
								},
							},
							{
								Id: &buildbucketpb.BuilderID{
									Project: "allowed-project",
									Bucket:  "bucket",
									Builder: "new-builder",
								},
							},
						},
					},
					BuilderSnapshots: []*milopb.BuilderSnapshot{
						{
							Builder: &buildbucketpb.BuilderID{
								Project: "allowed-project",
								Bucket:  "bucket",
								Builder: "builder",
							},
							Build: &buildbucketpb.Build{
								Id: 111111,
								Builder: &buildbucketpb.BuilderID{
									Project: "allowed-project",
									Bucket:  "bucket",
									Builder: "builder",
								},
								Status: buildbucketpb.Status_INFRA_FAILURE,
							},
						},
						{
							Builder: &buildbucketpb.BuilderID{
								Project: "allowed-project",
								Bucket:  "bucket",
								Builder: "new-builder",
							},
							Build: nil,
						},
					},
				},
			}))
			assert.Loosely(t, res.NextPageToken, should.NotBeEmpty)

			res, err = srv.QueryConsoleSnapshots(ctx, &milopb.QueryConsoleSnapshotsRequest{
				Predicate: &milopb.ConsolePredicate{
					Project: "allowed-project",
				},
				PageSize:  2,
				PageToken: res.NextPageToken,
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res.Snapshots, should.Resemble([]*milopb.ConsoleSnapshot{
				{
					Console: &projectconfigpb.Console{
						Realm: "allowed-project:@root",
						Id:    "con3",
						Builders: []*projectconfigpb.Builder{
							{
								Id: &buildbucketpb.BuilderID{
									Project: "other-allowed-project",
									Bucket:  "bucket",
									Builder: "builder",
								},
							},
						},
					},
					BuilderSnapshots: []*milopb.BuilderSnapshot{
						{
							Builder: &buildbucketpb.BuilderID{
								Project: "other-allowed-project",
								Bucket:  "bucket",
								Builder: "builder",
							},
							Build: &buildbucketpb.Build{
								Id: 111112,
								Builder: &buildbucketpb.BuilderID{
									Project: "other-allowed-project",
									Bucket:  "bucket",
									Builder: "builder",
								},
								Status: buildbucketpb.Status_SUCCESS,
							},
						},
					},
				},
			}))
			assert.Loosely(t, res.NextPageToken, should.BeEmpty)
		})

		t.Run(`query with builder predicate`, func(t *ftt.Test) {
			ctx := auth.WithState(ctx, &authtest.FakeState{Identity: "user", IdentityPermissions: perms})

			res, err := srv.QueryConsoleSnapshots(ctx, &milopb.QueryConsoleSnapshotsRequest{
				Predicate: &milopb.ConsolePredicate{
					Project: "allowed-project",
					Builder: &buildbucketpb.BuilderID{
						Project: "other-allowed-project",
						Bucket:  "bucket",
						Builder: "builder",
					},
				},
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res.Snapshots, should.Resemble([]*milopb.ConsoleSnapshot{
				{
					Console: &projectconfigpb.Console{
						Realm: "allowed-project:@root",
						Id:    "con1",
						Builders: []*projectconfigpb.Builder{
							{
								Id: &buildbucketpb.BuilderID{
									Project: "allowed-project",
									Bucket:  "bucket",
									Builder: "builder",
								},
							},
							{
								Id: &buildbucketpb.BuilderID{
									Project: "other-allowed-project",
									Bucket:  "bucket",
									Builder: "builder",
								},
							},
						},
					},
					BuilderSnapshots: []*milopb.BuilderSnapshot{
						{
							Builder: &buildbucketpb.BuilderID{
								Project: "allowed-project",
								Bucket:  "bucket",
								Builder: "builder",
							},
							Build: &buildbucketpb.Build{
								Id: 111111,
								Builder: &buildbucketpb.BuilderID{
									Project: "allowed-project",
									Bucket:  "bucket",
									Builder: "builder",
								},
								Status: buildbucketpb.Status_INFRA_FAILURE,
							},
						},
						{
							Builder: &buildbucketpb.BuilderID{
								Project: "other-allowed-project",
								Bucket:  "bucket",
								Builder: "builder",
							},
							Build: &buildbucketpb.Build{
								Id: 111112,
								Builder: &buildbucketpb.BuilderID{
									Project: "other-allowed-project",
									Bucket:  "bucket",
									Builder: "builder",
								},
								Status: buildbucketpb.Status_SUCCESS,
							},
						},
					},
				},
				{
					Console: &projectconfigpb.Console{
						Realm: "allowed-project:@root",
						Id:    "con3",
						Builders: []*projectconfigpb.Builder{
							{
								Id: &buildbucketpb.BuilderID{
									Project: "other-allowed-project",
									Bucket:  "bucket",
									Builder: "builder",
								},
							},
						},
					},
					BuilderSnapshots: []*milopb.BuilderSnapshot{
						{
							Builder: &buildbucketpb.BuilderID{
								Project: "other-allowed-project",
								Bucket:  "bucket",
								Builder: "builder",
							},
							Build: &buildbucketpb.Build{
								Id: 111112,
								Builder: &buildbucketpb.BuilderID{
									Project: "other-allowed-project",
									Bucket:  "bucket",
									Builder: "builder",
								},
								Status: buildbucketpb.Status_SUCCESS,
							},
						},
					},
				},
			}))
			assert.Loosely(t, res.NextPageToken, should.BeEmpty)
		})

		t.Run(`query forbidden project`, func(t *ftt.Test) {
			ctx := auth.WithState(ctx, &authtest.FakeState{Identity: "user", IdentityPermissions: perms})

			res, err := srv.QueryConsoleSnapshots(ctx, &milopb.QueryConsoleSnapshotsRequest{
				Predicate: &milopb.ConsolePredicate{
					Project: "forbidden-project",
				},
			})
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, grpcutil.Code(err), should.Equal(codes.PermissionDenied))
			assert.Loosely(t, res, should.BeNil)
		})

		t.Run(`query forbidden project with builder predicate`, func(t *ftt.Test) {
			ctx := auth.WithState(ctx, &authtest.FakeState{Identity: "user", IdentityPermissions: perms})

			res, err := srv.QueryConsoleSnapshots(ctx, &milopb.QueryConsoleSnapshotsRequest{
				Predicate: &milopb.ConsolePredicate{
					Project: "allowed-project",
					Builder: &buildbucketpb.BuilderID{
						Project: "forbidden-project",
						Bucket:  "bucket",
						Builder: "builder",
					},
				},
			})
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, grpcutil.Code(err), should.Equal(codes.PermissionDenied))
			assert.Loosely(t, res, should.BeNil)
		})

		t.Run(`resolve external consoles`, func(t *ftt.Test) {
			ctx := auth.WithState(ctx, &authtest.FakeState{Identity: "user", IdentityPermissions: perms})

			res, err := srv.QueryConsoleSnapshots(ctx, &milopb.QueryConsoleSnapshotsRequest{
				Predicate: &milopb.ConsolePredicate{
					Project: "project-with-external-ref",
				},
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res.Snapshots, should.Resemble([]*milopb.ConsoleSnapshot{
				{
					Console: &projectconfigpb.Console{
						Realm: "allowed-project:@root",
						Id:    "con1",
						Builders: []*projectconfigpb.Builder{
							{
								Id: &buildbucketpb.BuilderID{
									Project: "allowed-project",
									Bucket:  "bucket",
									Builder: "builder",
								},
							},
							{
								Id: &buildbucketpb.BuilderID{
									Project: "other-allowed-project",
									Bucket:  "bucket",
									Builder: "builder",
								},
							},
						},
					},
					BuilderSnapshots: []*milopb.BuilderSnapshot{
						{
							Builder: &buildbucketpb.BuilderID{
								Project: "allowed-project",
								Bucket:  "bucket",
								Builder: "builder",
							},
							Build: &buildbucketpb.Build{
								Id: 111111,
								Builder: &buildbucketpb.BuilderID{
									Project: "allowed-project",
									Bucket:  "bucket",
									Builder: "builder",
								},
								Status: buildbucketpb.Status_INFRA_FAILURE,
							},
						},
						{
							Builder: &buildbucketpb.BuilderID{
								Project: "other-allowed-project",
								Bucket:  "bucket",
								Builder: "builder",
							},
							Build: &buildbucketpb.Build{
								Id: 111112,
								Builder: &buildbucketpb.BuilderID{
									Project: "other-allowed-project",
									Bucket:  "bucket",
									Builder: "builder",
								},
								Status: buildbucketpb.Status_SUCCESS,
							},
						},
					},
				},
				{
					Console: &projectconfigpb.Console{
						Realm: "project-with-external-ref:@root",
						Id:    "con3",
						Builders: []*projectconfigpb.Builder{
							{
								Id: &buildbucketpb.BuilderID{
									Project: "project-with-external-ref",
									Bucket:  "bucket",
									Builder: "builder",
								},
							},
						},
					},
					BuilderSnapshots: []*milopb.BuilderSnapshot{
						{
							Builder: &buildbucketpb.BuilderID{
								Project: "project-with-external-ref",
								Bucket:  "bucket",
								Builder: "builder",
							},
							Build: &buildbucketpb.Build{
								Id: 111114,
								Builder: &buildbucketpb.BuilderID{
									Project: "project-with-external-ref",
									Bucket:  "bucket",
									Builder: "builder",
								},
								Status: buildbucketpb.Status_FAILURE,
							},
						},
					},
				},
			}))
			assert.Loosely(t, res.NextPageToken, should.BeEmpty)
		})
	})
}

func TestValidateQueryConsoleSnapshotsQuery(t *testing.T) {
	t.Parallel()
	ftt.Run(`TestValidateQueryConsoleSnapshotsRequest`, t, func(t *ftt.Test) {
		t.Run(`negative page size`, func(t *ftt.Test) {
			err := validateQueryConsoleSnapshotsRequest(&milopb.QueryConsoleSnapshotsRequest{
				Predicate: &milopb.ConsolePredicate{
					Project: "project",
				},
				PageSize: -1,
			})
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, err, should.ErrLike("page_size can not be negative"))
		})

		t.Run(`missing project`, func(t *ftt.Test) {
			err := validateQueryConsoleSnapshotsRequest(&milopb.QueryConsoleSnapshotsRequest{
				PageSize: 10,
			})
			assert.Loosely(t, err, should.ErrLike("predicate.project is required"))
		})

		t.Run(`valid`, func(t *ftt.Test) {
			err := validateQueryConsoleSnapshotsRequest(&milopb.QueryConsoleSnapshotsRequest{
				Predicate: &milopb.ConsolePredicate{
					Project: "project",
					Builder: &buildbucketpb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
				},
				PageSize: 10,
			})
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run(`valid with only project`, func(t *ftt.Test) {
			err := validateQueryConsoleSnapshotsRequest(&milopb.QueryConsoleSnapshotsRequest{
				Predicate: &milopb.ConsolePredicate{
					Project: "project",
				},
			})
			assert.Loosely(t, err, should.BeNil)
		})
	})
}
