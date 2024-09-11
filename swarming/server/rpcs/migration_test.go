// Copyright 2024 The LUCI Authors.
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

package rpcs

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"testing"

	"google.golang.org/grpc/metadata"

	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/prpctest"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/server/secrets"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	configpb "go.chromium.org/luci/swarming/proto/config"
	"go.chromium.org/luci/swarming/server/cfg/cfgtest"
	"go.chromium.org/luci/swarming/server/cursor"
	"go.chromium.org/luci/swarming/server/cursor/cursorpb"
)

func TestConfigureMigration(t *testing.T) {
	t.Parallel()

	ctx := memory.Use(context.Background())
	ctx = secrets.GeneratePrimaryTinkAEADForTest(ctx)
	ctx = mathrand.Set(ctx, rand.New(rand.NewSource(123)))

	cfg := cfgtest.MockConfigs(ctx, &cfgtest.MockedConfigs{
		Settings: &configpb.SettingsCfg{
			TrafficMigration: &configpb.TrafficMigration{
				Routes: []*configpb.TrafficMigration_Route{
					{Name: "/prpc/swarming.v2.Bots/DeleteBot", RouteToGoPercent: 100},
					{Name: "/prpc/swarming.v2.Bots/ListBots", RouteToGoPercent: 20},
				},
			},
		},
	})

	var requests []string
	var m sync.Mutex

	request := func(name string) {
		m.Lock()
		defer m.Unlock()
		requests = append(requests, name)
	}
	seen := func() []string {
		m.Lock()
		defer m.Unlock()
		seen := requests
		requests = nil
		return seen
	}

	goBotsService := &fakeBotsService{
		kind:    "go",
		request: func(_ context.Context, name string) { request(name) },
	}
	goPrpcSrv := &prpctest.Server{}
	apipb.RegisterBotsServer(goPrpcSrv, goBotsService)

	pyBotsService := &fakeBotsService{
		kind: "py",
		request: func(ctx context.Context, name string) {
			md, _ := metadata.FromIncomingContext(ctx)
			if val := md.Get("X-Routed-From-Go"); len(val) == 0 || val[0] != "1" {
				t.Fatalf("wrong X-Routed-From-Go")
			}
			request(name)
		},
	}
	pyPrpcSrv := &prpctest.Server{}
	apipb.RegisterBotsServer(pyPrpcSrv, pyBotsService)

	pyPrpcSrv.Start(ctx) // to get pyPrpcSrv.HTTP.URL
	defer pyPrpcSrv.Close()

	ConfigureMigration(&goPrpcSrv.Server, cfg, pyPrpcSrv.HTTP.URL)

	goPrpcSrv.Start(ctx)
	defer goPrpcSrv.Close()

	goPrpcClient, err := goPrpcSrv.NewClient()
	if err != nil {
		t.Fatal(err)
	}
	botsClient := apipb.NewBotsClient(goPrpcClient)

	ftt.Run("Sends to Python by default", t, func(t *ftt.Test) {
		// Send a bunch to make sure it is not just unlucky random routing.
		for i := 0; i < 10; i++ {
			_, err := botsClient.GetBot(ctx, &apipb.BotRequest{})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, seen(), should.Resemble([]string{"py:GetBot"}))
		}
	})

	ftt.Run("RouteToGoPercent == 100 => sends all requests to Go", t, func(t *ftt.Test) {
		// Send a bunch to make sure it is not just unlucky random routing.
		for i := 0; i < 10; i++ {
			_, err := botsClient.DeleteBot(ctx, &apipb.BotRequest{})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, seen(), should.Resemble([]string{"go:DeleteBot"}))
		}
	})

	ftt.Run("X-Route-To header works", t, func(t *ftt.Test) {
		_, err := botsClient.GetBot(
			metadata.NewOutgoingContext(ctx, metadata.Pairs("x-route-to", "go")),
			&apipb.BotRequest{},
		)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, seen(), should.Resemble([]string{"go:GetBot"}))

		_, err = botsClient.DeleteBot(
			metadata.NewOutgoingContext(ctx, metadata.Pairs("x-route-to", "py")),
			&apipb.BotRequest{},
		)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, seen(), should.Resemble([]string{"py:DeleteBot"}))
	})

	ftt.Run("X-Routed-From-Go disables proxying to break the loop", t, func(t *ftt.Test) {
		_, err := botsClient.GetBot(
			metadata.NewOutgoingContext(ctx, metadata.Pairs("x-routed-from-go", "1")),
			&apipb.BotRequest{},
		)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, seen(), should.Resemble([]string{"go:GetBot"}))
	})

	ftt.Run("RouteToGoPercent == 20 => some requests are sent to Go, some to Python", t, func(t *ftt.Test) {
		for i := 0; i < 10; i++ {
			_, err := botsClient.ListBots(ctx, &apipb.BotsRequest{})
			assert.Loosely(t, err, should.BeNil)
		}
		assert.Loosely(t, seen(), should.Resemble([]string{
			"py:ListBots",
			"py:ListBots",
			"go:ListBots",
			"py:ListBots",
			"py:ListBots",
			"py:ListBots",
			"py:ListBots",
			"go:ListBots",
			"py:ListBots",
			"go:ListBots",
		}))
	})

	ftt.Run("Go pagination cursor => requests are sent to Go", t, func(t *ftt.Test) {
		cur, err := cursor.Encode(ctx, cursorpb.RequestKind_LIST_BOTS, &cursorpb.BotsCursor{
			LastBotId: "zzz",
		})
		assert.Loosely(t, err, should.BeNil)
		for i := 0; i < 10; i++ {
			_, err := botsClient.ListBots(ctx, &apipb.BotsRequest{Cursor: cur})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, seen(), should.Resemble([]string{"go:ListBots"}))
		}
	})

	ftt.Run("Non-go pagination cursor => requests are sent to Python", t, func(t *ftt.Test) {
		assert.Loosely(t, err, should.BeNil)
		for i := 0; i < 10; i++ {
			_, err := botsClient.ListBots(ctx, &apipb.BotsRequest{Cursor: "i-am-not-a-go-cursor"})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, seen(), should.Resemble([]string{"py:ListBots"}))
		}
	})
}

type fakeBotsService struct {
	apipb.UnimplementedBotsServer

	kind    string // "py" or "go"
	request func(ctx context.Context, name string)
}

func (s *fakeBotsService) GetBot(ctx context.Context, req *apipb.BotRequest) (*apipb.BotInfo, error) {
	s.request(ctx, fmt.Sprintf("%s:GetBot", s.kind))
	return &apipb.BotInfo{}, nil
}

func (s *fakeBotsService) DeleteBot(ctx context.Context, req *apipb.BotRequest) (*apipb.DeleteResponse, error) {
	s.request(ctx, fmt.Sprintf("%s:DeleteBot", s.kind))
	return &apipb.DeleteResponse{}, nil
}

func (s *fakeBotsService) ListBots(ctx context.Context, req *apipb.BotsRequest) (*apipb.BotInfoListResponse, error) {
	s.request(ctx, fmt.Sprintf("%s:ListBots", s.kind))
	return &apipb.BotInfoListResponse{}, nil
}
