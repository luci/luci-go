// Copyright 2016 The LUCI Authors.
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

package coordinator

import (
	"context"
	"testing"

	"github.com/golang/protobuf/proto"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	cfglib "go.chromium.org/luci/config"
	"go.chromium.org/luci/config/cfgclient"
	cfgmem "go.chromium.org/luci/config/impl/memory"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/logdog/api/config/svcconfig"
	"go.chromium.org/luci/logdog/server/config"
)

func TestWithProjectNamespace(t *testing.T) {
	t.Parallel()

	ftt.Run(`A testing environment`, t, func(t *ftt.Test) {
		ctx := context.Background()
		ctx = memory.Use(ctx)

		ctx = withProjectConfigs(ctx, map[string]*svcconfig.ProjectConfig{
			"existing": {ArchiveGsBucket: "some-bucket"},
		})

		t.Run(`Entering existing project`, func(t *ftt.Test) {
			assert.Loosely(t, WithProjectNamespace(&ctx, "existing"), should.BeNil)
			assert.Loosely(t, Project(ctx), should.Equal("existing"))
			cfg, err := ProjectConfig(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, cfg.ArchiveGsBucket, should.Equal("some-bucket"))
		})

		t.Run(`Entering non-existing project`, func(t *ftt.Test) {
			t.Run(`Anonymous`, func(t *ftt.Test) {
				err := WithProjectNamespace(&ctx, "non-existing")
				assert.Loosely(t, err, grpccode.ShouldBe(codes.Unauthenticated))
			})
			t.Run(`Non-anonymous`, func(t *ftt.Test) {
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity: "user:someone@example.com",
				})
				err := WithProjectNamespace(&ctx, "non-existing")
				assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
			})
		})
	})
}

// withProjectConfigs configures config.Store in the context.
func withProjectConfigs(ctx context.Context, p map[string]*svcconfig.ProjectConfig) context.Context {
	// Prep text config files in memory.
	configs := make(map[cfglib.Set]cfgmem.Files, len(p))
	for projectID, cfg := range p {
		configs[cfglib.MustProjectSet(projectID)] = cfgmem.Files{
			"${appid}.cfg": proto.MarshalTextString(cfg),
		}
	}
	// Install in-memory LUCI config "client" that serves them.
	ctx = cfgclient.Use(ctx, cfgmem.New(configs))
	// Sync them into the datastore.
	config.Sync(ctx)
	// Make them available to handlers.
	return config.WithStore(ctx, &config.Store{NoCache: true})
}
