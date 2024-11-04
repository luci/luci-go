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

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/structpb"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/errors"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	"go.chromium.org/luci/swarming/server/validate"
)

const (
	// Default value to wait for pings from bot.
	defaultBotPingTolerance = 1200
)

// ValidateConfigs implements bbpb.TaskBackendServer.
func (srv *TaskBackend) ValidateConfigs(ctx context.Context, req *bbpb.ValidateConfigsRequest) (*bbpb.ValidateConfigsResponse, error) {
	if err := srv.CheckBuildbucket(ctx); err != nil {
		return nil, err
	}

	l := len(req.GetConfigs())
	if l == 0 {
		return &bbpb.ValidateConfigsResponse{}, nil
	}

	res := &bbpb.ValidateConfigsResponse{}
	addErrs := func(i int, errs ...error) {
		for _, err := range errs {
			ed := &bbpb.ValidateConfigsResponse_ErrorDetail{
				Index: int32(i),
				Error: err.Error(),
			}
			res.ConfigErrors = append(res.ConfigErrors, ed)
		}
	}
	for i, cfgCtx := range req.Configs {
		config, err := ingestBackendConfigWithDefaults(cfgCtx.GetConfigJson())
		if err != nil {
			addErrs(i, err)
		} else {
			addErrs(i, validateBackendConfig(config)...)
		}
	}
	return res, nil
}

func ingestBackendConfigWithDefaults(cfgStruct *structpb.Struct) (*apipb.SwarmingTaskBackendConfig, error) {
	cfgJSON, err := cfgStruct.MarshalJSON()
	if err != nil {
		return nil, err
	}
	config := &apipb.SwarmingTaskBackendConfig{}
	err = protojson.Unmarshal(cfgJSON, config)
	if err != nil {
		return nil, err
	}

	// Apply defaults.
	if config.BotPingTolerance == 0 {
		config.BotPingTolerance = defaultBotPingTolerance
	}
	return config, nil
}

// validateBackendConfig does basic validates on a `config`.
//
// It is used by ValidateConfigs RPC to validate the backend configs from
// bbpb.BuilderConfig.
//
// It'll also be used by RunTask RPC to do preliminary validations. And then
// some additional checks will be done on
// * fields are only populated at Buildbucket build creation time:
//   - wait_for_capacity (to be deprecated with RBE scheduling)
//   - agent_binary_cipd_*
//
// * fields are only required by RunTask:
//   - priority
//   - service_account
//   - agent_binary_cipd_*
func validateBackendConfig(cfg *apipb.SwarmingTaskBackendConfig) []error {
	var errs []error
	if cfg.GetPriority() != 0 {
		if err := validate.Priority(cfg.Priority); err != nil {
			errs = append(errs, errors.Annotate(err, "priority").Err())
		}
	}

	if cfg.GetBotPingTolerance() != 0 {
		if err := validate.BotPingTolerance(cfg.BotPingTolerance); err != nil {
			errs = append(errs, errors.Annotate(err, "bot_ping_tolerance").Err())
		}
	}

	if cfg.GetServiceAccount() != "" {
		if err := validate.ServiceAccount(cfg.ServiceAccount); err != nil {
			errs = append(errs, errors.Annotate(err, "service_account").Err())
		}
	}

	for i, t := range cfg.Tags {
		if err := validate.Tag(t); err != nil {
			errs = append(errs, errors.Annotate(err, "tag %d", i).Err())
		}
	}
	return errs
}
