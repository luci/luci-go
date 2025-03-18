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
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/secrets"

	"go.chromium.org/luci/milo/internal/projectconfig"
	"go.chromium.org/luci/milo/internal/testutils"
	"go.chromium.org/luci/milo/internal/utils"
	projectconfigpb "go.chromium.org/luci/milo/proto/projectconfig"
	milopb "go.chromium.org/luci/milo/proto/v1"
)

func TestQueryConsoles(t *testing.T) {
	t.Parallel()
	ftt.Run(`TestQueryConsoles`, t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		ctx = testutils.SetUpTestGlobalCache(ctx)
		ctx = secrets.GeneratePrimaryTinkAEADForTest(ctx)

		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)

		err := datastore.Put(ctx, []*projectconfig.Project{
			{
				ID:  "allowed-project",
				ACL: projectconfig.ACL{Identities: []identity.Identity{"user"}},
			},
			{
				ID:  "other-allowed-project",
				ACL: projectconfig.ACL{Identities: []identity.Identity{"user"}},
			},
		})
		assert.Loosely(t, err, should.BeNil)

		consoleIDs := []*projectconfig.ConsoleID{
			{

				Project: "allowed-project",
				ID:      "con1",
			},
			{
				Project: "allowed-project",
				ID:      "con2",
			},
			{
				Project: "allowed-project",
				ID:      "con3",
			},
			{
				Project: "other-allowed-project",
				ID:      "con4",
			},
			{
				Project: "forbidden-project",
				ID:      "con5",
			},
		}
		consoleBuilders := [][]*buildbucketpb.BuilderID{
			{
				{
					Project: "allowed-project",
					Bucket:  "bucket1",
					Builder: "builder1",
				},
				{
					Project: "allowed-project",
					Bucket:  "bucket2",
					Builder: "builder2",
				},
				{
					Project: "allowed-project",
					Bucket:  "bucket3",
					Builder: "builder3",
				},
				{
					Project: "forbidden-project",
					Bucket:  "bucket",
					Builder: "builder",
				},
			},
			{
				{
					Project: "allowed-project",
					Bucket:  "bucket1",
					Builder: "builder1",
				},
				{
					Project: "allowed-project",
					Bucket:  "bucket2",
					Builder: "builder2",
				},
			},
			{
				{
					Project: "allowed-project",
					Bucket:  "bucket2",
					Builder: "builder2",
				},
				{
					Project: "allowed-project",
					Bucket:  "bucket3",
					Builder: "builder3",
				},
			},
			{
				{
					Project: "allowed-project",
					Bucket:  "bucket1",
					Builder: "builder1",
				},
				{
					Project: "allowed-project",
					Bucket:  "bucket3",
					Builder: "builder3",
				},
			},
			{
				{
					Project: "allowed-project",
					Bucket:  "bucket1",
					Builder: "builder1",
				},
				{
					Project: "allowed-project",
					Bucket:  "bucket3",
					Builder: "builder3",
				},
			},
		}

		consoles := make([]*projectconfig.Console, 0, len(consoleIDs))
		for i, conID := range consoleIDs {
			console := conID.SetID(ctx, nil)
			console.Builders = make([]string, 0, len(consoleBuilders[i]))
			console.Def = &projectconfigpb.Console{
				Builders: make([]*projectconfigpb.Builder, 0, len(consoleBuilders[i])),
			}
			for _, builderID := range consoleBuilders[i] {
				legacyID := utils.LegacyBuilderIDString(builderID)
				console.Builders = append(console.Builders, legacyID)
				console.Def.Builders = append(console.Def.Builders, &projectconfigpb.Builder{Name: legacyID})
			}
			consoles = append(consoles, console)
		}

		err = datastore.Put(ctx, consoles)
		assert.Loosely(t, err, should.BeNil)

		srv := &MiloInternalService{}

		t.Run(`e2e`, func(t *ftt.Test) {
			ctx := auth.WithState(ctx, &authtest.FakeState{Identity: "user"})

			res, err := srv.QueryConsoles(ctx, &milopb.QueryConsolesRequest{
				Predicate: &milopb.ConsolePredicate{
					Builder: &buildbucketpb.BuilderID{
						Project: "allowed-project",
						Bucket:  "bucket1",
						Builder: "builder1",
					},
				},
				PageSize: 2,
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res.Consoles, should.Match([]*projectconfigpb.Console{
				{
					Id:    "con1",
					Realm: "allowed-project:@root",
				},
				{
					Id:    "con2",
					Realm: "allowed-project:@root",
				},
			}))
			assert.Loosely(t, res.NextPageToken, should.NotBeEmpty)

			res, err = srv.QueryConsoles(ctx, &milopb.QueryConsolesRequest{
				Predicate: &milopb.ConsolePredicate{
					Builder: &buildbucketpb.BuilderID{
						Project: "allowed-project",
						Bucket:  "bucket1",
						Builder: "builder1",
					},
				},
				PageSize:  2,
				PageToken: res.NextPageToken,
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res.Consoles, should.Match([]*projectconfigpb.Console{
				{
					Id:    "con4",
					Realm: "other-allowed-project:@root",
				},
			}))
			assert.Loosely(t, res.NextPageToken, should.BeEmpty)
		})

		t.Run(`query project`, func(t *ftt.Test) {
			ctx := auth.WithState(ctx, &authtest.FakeState{Identity: "user"})

			res, err := srv.QueryConsoles(ctx, &milopb.QueryConsolesRequest{
				Predicate: &milopb.ConsolePredicate{
					Project: "allowed-project",
				},
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res.Consoles, should.Match([]*projectconfigpb.Console{
				{
					Id:    "con1",
					Realm: "allowed-project:@root",
				},
				{
					Id:    "con2",
					Realm: "allowed-project:@root",
				},
				{
					Id:    "con3",
					Realm: "allowed-project:@root",
				},
			}))
		})

		t.Run(`query project and builder`, func(t *ftt.Test) {
			ctx := auth.WithState(ctx, &authtest.FakeState{Identity: "user"})

			res, err := srv.QueryConsoles(ctx, &milopb.QueryConsolesRequest{
				Predicate: &milopb.ConsolePredicate{
					Project: "allowed-project",
					Builder: &buildbucketpb.BuilderID{
						Project: "allowed-project",
						Bucket:  "bucket1",
						Builder: "builder1",
					},
				},
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res.Consoles, should.Match([]*projectconfigpb.Console{
				{
					Id:    "con1",
					Realm: "allowed-project:@root",
				},
				{
					Id:    "con2",
					Realm: "allowed-project:@root",
				},
			}))
		})

		t.Run(`query all`, func(t *ftt.Test) {
			ctx := auth.WithState(ctx, &authtest.FakeState{Identity: "user"})

			res, err := srv.QueryConsoles(ctx, &milopb.QueryConsolesRequest{})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res.Consoles, should.Match([]*projectconfigpb.Console{
				{
					Id:    "con1",
					Realm: "allowed-project:@root",
				},
				{
					Id:    "con2",
					Realm: "allowed-project:@root",
				},
				{
					Id:    "con3",
					Realm: "allowed-project:@root",
				},
				{
					Id:    "con4",
					Realm: "other-allowed-project:@root",
				},
			}))
		})

		t.Run(`query forbidden project`, func(t *ftt.Test) {
			ctx := auth.WithState(ctx, &authtest.FakeState{Identity: "user"})

			res, err := srv.QueryConsoles(ctx, &milopb.QueryConsolesRequest{
				Predicate: &milopb.ConsolePredicate{
					Project: "forbidden-project",
				},
				PageSize: 2,
			})
			assert.Loosely(t, res, should.BeNil)
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, grpcutil.Code(err), should.Equal(codes.PermissionDenied))
		})

		t.Run(`query forbidden project with builder predicate`, func(t *ftt.Test) {
			ctx := auth.WithState(ctx, &authtest.FakeState{Identity: "user"})

			res, err := srv.QueryConsoles(ctx, &milopb.QueryConsolesRequest{
				Predicate: &milopb.ConsolePredicate{
					Builder: &buildbucketpb.BuilderID{
						Project: "forbidden-project",
						Bucket:  "bucket1",
						Builder: "builder1",
					},
				},
				PageSize: 2,
			})
			assert.Loosely(t, res, should.BeNil)
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, grpcutil.Code(err), should.Equal(codes.PermissionDenied))
		})
	})
}

func TestValidateQueryConsolesQuery(t *testing.T) {
	t.Parallel()
	ftt.Run(`TestValidateQueryConsolesRequest`, t, func(t *ftt.Test) {
		t.Run(`negative page size`, func(t *ftt.Test) {
			err := validatesQueryConsolesRequest(&milopb.QueryConsolesRequest{
				Predicate: &milopb.ConsolePredicate{
					Builder: &buildbucketpb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
				},
				PageSize: -1,
			})
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, err, should.ErrLike("page_size can not be negative"))
		})

		t.Run(`valid`, func(t *ftt.Test) {
			err := validatesQueryConsolesRequest(&milopb.QueryConsolesRequest{
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

		t.Run(`valid with no predicate`, func(t *ftt.Test) {
			err := validatesQueryConsolesRequest(&milopb.QueryConsolesRequest{
				PageSize: 10,
			})
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run(`valid with only project`, func(t *ftt.Test) {
			err := validatesQueryConsolesRequest(&milopb.QueryConsolesRequest{
				Predicate: &milopb.ConsolePredicate{
					Project: "project",
				},
			})
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run(`valid with only builder`, func(t *ftt.Test) {
			err := validatesQueryConsolesRequest(&milopb.QueryConsolesRequest{
				Predicate: &milopb.ConsolePredicate{
					Builder: &buildbucketpb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
				},
			})
			assert.Loosely(t, err, should.BeNil)
		})
	})
}
