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

	. "github.com/smartystreets/goconvey/convey"
	"go.chromium.org/luci/auth/identity"
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	. "go.chromium.org/luci/common/testing/assertions"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/milo/internal/projectconfig"
	"go.chromium.org/luci/milo/internal/testutils"
	"go.chromium.org/luci/milo/internal/utils"
	projectconfigpb "go.chromium.org/luci/milo/proto/projectconfig"
	milopb "go.chromium.org/luci/milo/proto/v1"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/secrets"
	"google.golang.org/grpc/codes"
)

func TestQueryConsoles(t *testing.T) {
	t.Parallel()
	Convey(`TestQueryConsoles`, t, func() {
		ctx := memory.Use(context.Background())
		ctx = testutils.SetUpTestGlobalCache(ctx)
		ctx = secrets.GeneratePrimaryTinkAEADForTest(ctx)

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
		So(err, ShouldBeNil)

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
			console.Def = projectconfigpb.Console{
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
		So(err, ShouldBeNil)

		srv := &MiloInternalService{}

		Convey(`e2e`, func() {
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
			So(err, ShouldBeNil)
			So(res.Consoles, ShouldResembleProto, []*projectconfigpb.Console{
				{
					Id:    "con1",
					Realm: "allowed-project:@root",
				},
				{
					Id:    "con2",
					Realm: "allowed-project:@root",
				},
			})
			So(res.NextPageToken, ShouldNotBeEmpty)

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
			So(err, ShouldBeNil)
			So(res.Consoles, ShouldResembleProto, []*projectconfigpb.Console{
				{
					Id:    "con4",
					Realm: "other-allowed-project:@root",
				},
			})
			So(res.NextPageToken, ShouldBeEmpty)
		})

		Convey(`query forbidden project`, func() {
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
			So(res, ShouldBeNil)
			So(err, ShouldNotBeNil)
			So(grpcutil.Code(err), ShouldEqual, codes.PermissionDenied)
		})
	})
}

func TestValidateQueryConsolesQuery(t *testing.T) {
	t.Parallel()
	Convey(`TestValidateQueryConsolesRequest`, t, func() {
		Convey(`no predicate`, func() {
			err := validatesQueryConsolesRequest(&milopb.QueryConsolesRequest{})
			So(err, ShouldNotBeNil)
			So(err, ShouldErrLike, "predicate: builder must be specified")
		})

		Convey(`negative page size`, func() {
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
			So(err, ShouldNotBeNil)
			So(err, ShouldErrLike, "page_size can not be negative")
		})

		Convey(`valid`, func() {
			err := validatesQueryConsolesRequest(&milopb.QueryConsolesRequest{
				Predicate: &milopb.ConsolePredicate{
					Builder: &buildbucketpb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
				},
				PageSize: 10,
			})
			So(err, ShouldBeNil)
		})
	})
}
