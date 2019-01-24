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

package base

import (
	"context"

	config "go.chromium.org/luci/common/api/luci_config/config/v1"

	"go.chromium.org/luci/lucicfg"
)

// ConfigServiceFactory returns LUCI Config RPC client that sends requests
// to the given host.
//
// This is usually just subcommand.ConfigService.
type ConfigServiceFactory func(ctx context.Context, host string) (*config.Service, error)

// ValidateConfigs sends the config set to LUCI Config for validation.
//
// It is a common part of subcommands that validate configs.
//
// Dumps validation errors to the logger. Returns an error if the config set is
// invalid or the RPC call itself failed.
//
// Assumes caller has already verified ConfigServiceHost and ConfigSet meta
// fields are set.
func ValidateConfigs(ctx context.Context, cs lucicfg.ConfigSet, meta *lucicfg.Meta, svc ConfigServiceFactory) (*lucicfg.ValidationResult, error) {
	srv, err := svc(ctx, meta.ConfigServiceHost)
	if err != nil {
		return nil, err
	}
	result, err := cs.Validate(ctx, meta.ConfigSet, lucicfg.RemoteValidator(srv))
	if err != nil {
		return nil, err
	}
	result.Log(ctx)
	return result, result.OverallError(meta.FailOnWarnings)
}
