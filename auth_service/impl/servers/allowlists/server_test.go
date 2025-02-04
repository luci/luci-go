// Copyright 2021 The LUCI Authors.
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

package allowlists

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"

	"go.chromium.org/luci/auth_service/api/configspb"
	"go.chromium.org/luci/auth_service/api/rpcpb"
	"go.chromium.org/luci/auth_service/impl/model"
	"go.chromium.org/luci/auth_service/internal/configs/srvcfg/allowlistcfg"
)

func TestAllowlistsServer(t *testing.T) {
	t.Parallel()
	srv := Server{}
	createdTime := time.Date(2021, time.September, 16, 15, 20, 0, 0, time.UTC)

	ftt.Run("GetAllowlist RPC call", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())

		request := &rpcpb.GetAllowlistRequest{
			Name: "test-allowlist",
		}

		_, err := srv.GetAllowlist(ctx, request)
		assert.Loosely(t, err, grpccode.ShouldBe(codes.NotFound))

		// Allowlist built from model.AuthIPAllowlist definition.
		assert.Loosely(t, datastore.Put(ctx,
			&model.AuthIPAllowlist{
				AuthVersionedEntityMixin: model.AuthVersionedEntityMixin{},
				Parent:                   model.RootKey(ctx),
				ID:                       "test-allowlist",
				Subnets: []string{
					"127.0.0.1/24",
					"127.0.0.127/24",
				},
				Description: "This is a test allowlist.",
				CreatedTS:   createdTime,
				CreatedBy:   "user:test-user-1",
			}), should.BeNil)

		expectedResponse := &rpcpb.Allowlist{
			Name: "test-allowlist",
			Subnets: []string{
				"127.0.0.1/24",
				"127.0.0.127/24",
			},
			Description: "This is a test allowlist.",
			CreatedTs:   timestamppb.New(createdTime),
			CreatedBy:   "user:test-user-1",
		}

		actualResponse, err := srv.GetAllowlist(ctx, request)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, actualResponse, should.Match(expectedResponse))
	})

	ftt.Run("ListAllowlists RPC call", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())

		assert.Loosely(t, datastore.Put(ctx,
			&model.AuthIPAllowlist{
				AuthVersionedEntityMixin: model.AuthVersionedEntityMixin{},
				ID:                       "z-test-allowlist",
				Parent:                   model.RootKey(ctx),
				Subnets: []string{
					"127.0.0.1/24",
					"127.0.0.127/24",
				},
				Description: "This is a test allowlist, should show up last.",
				CreatedTS:   createdTime,
				CreatedBy:   "user:test-user-2",
			},
			&model.AuthIPAllowlist{
				AuthVersionedEntityMixin: model.AuthVersionedEntityMixin{},
				ID:                       "a-test-allowlist",
				Parent:                   model.RootKey(ctx),
				Subnets: []string{
					"0.0.0.0/0",
				},
				Description: "This is a test allowlist, should show up first.",
				CreatedTS:   createdTime,
				CreatedBy:   "user:test-user-1",
			},
			&model.AuthIPAllowlist{
				AuthVersionedEntityMixin: model.AuthVersionedEntityMixin{},
				ID:                       "test-allowlist",
				Parent:                   model.RootKey(ctx),
				Subnets:                  []string{},
				Description:              "This is a test allowlist, should show up second.",
				CreatedTS:                createdTime,
				CreatedBy:                "user:test-user-3",
			}), should.BeNil)

		_, err := srv.ListAllowlists(ctx, &emptypb.Empty{})
		assert.Loosely(t, err, grpccode.ShouldBe(codes.Internal))
		assert.Loosely(t, err, should.ErrLike("failed to get config metadata"))

		// Set up the allowlist config and its metadata.
		testConfig := &configspb.IPAllowlistConfig{}
		testConfigMetadata := &config.Meta{
			Path:     "ip_allowlist.cfg",
			Revision: "123abc",
			ViewURL:  "https://example.com/config/revision/123abc",
		}
		assert.Loosely(t, allowlistcfg.SetConfigWithMetadata(ctx, testConfig, testConfigMetadata), should.BeNil)

		// Expected response, build with pb.
		expectedAllowlists := &rpcpb.ListAllowlistsResponse{
			Allowlists: []*rpcpb.Allowlist{
				{
					Name: "a-test-allowlist",
					Subnets: []string{
						"0.0.0.0/0",
					},
					Description: "This is a test allowlist, should show up first.",
					CreatedTs:   timestamppb.New(createdTime),
					CreatedBy:   "user:test-user-1",
				},
				{
					Name:        "test-allowlist",
					Subnets:     []string{},
					Description: "This is a test allowlist, should show up second.",
					CreatedTs:   timestamppb.New(createdTime),
					CreatedBy:   "user:test-user-3",
				},
				{
					Name: "z-test-allowlist",
					Subnets: []string{
						"127.0.0.1/24",
						"127.0.0.127/24",
					},
					Description: "This is a test allowlist, should show up last.",
					CreatedTs:   timestamppb.New(createdTime),
					CreatedBy:   "user:test-user-2",
				},
			},
			ConfigViewUrl:  testConfigMetadata.ViewURL,
			ConfigRevision: testConfigMetadata.Revision,
		}

		actualResponse, err := srv.ListAllowlists(ctx, &emptypb.Empty{})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, expectedAllowlists.Allowlists, should.Match(actualResponse.Allowlists))
	})
}
