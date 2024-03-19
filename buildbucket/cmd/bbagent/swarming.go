// Copyright 2019 The LUCI Authors.
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

package main

import (
	"context"

	"google.golang.org/protobuf/proto"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/system/environ"
	"go.chromium.org/luci/lucictx"
	"go.chromium.org/luci/swarming/client/swarming"
)

// readBuildSecrets reads BuildSecrets message from swarming secret bytes.
func readBuildSecrets(ctx context.Context) (*bbpb.BuildSecrets, error) {
	swarming := lucictx.GetSwarming(ctx)
	if swarming == nil {
		return nil, errors.Reason("no swarming secret bytes; is this a Swarming Task with secret bytes?").Err()
	}

	secrets := &bbpb.BuildSecrets{}
	if err := proto.Unmarshal(swarming.SecretBytes, secrets); err != nil {
		return nil, errors.Annotate(err, "failed to read BuildSecrets message from swarming secret bytes").Err()
	}
	return secrets, nil
}

// populateSwarmingInfoFromEnv populates part of missing fields under
// `build.infra.swarming` using values from `SWARMING_*` environment
// variables.
func populateSwarmingInfoFromEnv(build *bbpb.Build, env environ.Env) {
	if build.Infra == nil {
		build.Infra = &bbpb.BuildInfra{}
	}
	if build.Infra.Swarming == nil {
		build.Infra.Swarming = &bbpb.BuildInfra_Swarming{}
	}

	swarm := build.Infra.Swarming
	if v, ok := env.Lookup(swarming.ServerEnvVar); ok && swarm.Hostname == "" {
		swarm.Hostname = v
	}
	if v, ok := env.Lookup(swarming.TaskIDEnvVar); ok && swarm.TaskId == "" {
		swarm.TaskId = v
	}
}
