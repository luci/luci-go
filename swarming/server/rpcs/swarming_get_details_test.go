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
	"testing"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	configpb "go.chromium.org/luci/swarming/proto/config"
)

func TestGetDetails(t *testing.T) {
	t.Parallel()

	ctx := memory.Use(context.Background())

	state := NewMockedRequestState()
	state.Configs.MockBotPackage("latest", map[string]string{"file": "body"})
	state.Configs.Settings.DisplayServerUrlTemplate = "https://display-server-url/%s"
	state.Configs.Settings.Cas = &configpb.CASSettings{ViewerServer: "https://cas-viewer-url"}

	srv := SwarmingServer{ServerVersion: "server-version"}

	ftt.Run("Authorized", t, func(t *ftt.Test) {
		state := state.SetCaller(AdminFakeCaller)
		resp, err := srv.GetDetails(MockRequestState(ctx, state), nil)
		assert.NoErr(t, err)
		assert.Loosely(t, resp, should.Resemble(&apipb.ServerDetails{
			ServerVersion:            "server-version",
			BotVersion:               "5a125a79c5acbb53caf1c88455a5142c86cca5fde92b61a7049f8dd9068d4ca4",
			DisplayServerUrlTemplate: "https://display-server-url/%s",
			CasViewerServer:          "https://cas-viewer-url",
		}))
	})

	ftt.Run("Not authorized", t, func(t *ftt.Test) {
		_, err := srv.GetDetails(MockRequestState(ctx, state), nil)
		assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
	})
}
