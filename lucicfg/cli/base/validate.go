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
	"sync"

	config "go.chromium.org/luci/common/api/luci_config/config/v1"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/lucicfg"
)

// ConfigServiceFactory returns LUCI Config RPC client that sends requests
// to the given host.
//
// This is usually just subcommand.ConfigService.
type ConfigServiceFactory func(ctx context.Context, host string) (*config.Service, error)

// ValidateOutput splits the output into 0 or more config sets and sends them
// for validation to LUCI Config.
//
// It is a common part of subcommands that validate configs.
//
// It is allowed for 'host' to be empty. This causes a warning and
// the validation is skipped.
//
// If failOnWarn is true, treat warnings from LUCI Config as errors.
//
// Dumps validation errors to the logger. In addition to detailed validation
// results, also returns a multi-error with all validation and RPC errors.
func ValidateOutput(ctx context.Context, output lucicfg.Output, svc ConfigServiceFactory, host string, failOnWarns bool) ([]*lucicfg.ValidationResult, error) {
	configSets := output.ConfigSets()
	if len(configSets) == 0 {
		return nil, nil // nothing to validate
	}

	// Log the warning only if there were some config sets we needed to validate.
	if host == "" {
		logging.Warningf(ctx, "Config service host is not set, skipping validation against LUCI Config service")
		return nil, nil
	}

	srv, err := svc(ctx, host)
	if err != nil {
		return nil, err
	}
	validator := lucicfg.RemoteValidator(srv)

	// Validate all config sets in parallel.
	results := make([]*lucicfg.ValidationResult, len(configSets))
	wg := sync.WaitGroup{}
	wg.Add(len(configSets))
	for i, cs := range configSets {
		i, cs := i, cs
		go func() {
			results[i] = cs.Validate(ctx, validator)
			wg.Done()
		}()
	}
	wg.Wait()

	// Log all messages, assemble the final verdict. Note that OverallError
	// mutates r.Failed.
	var merr errors.MultiError
	for _, r := range results {
		r.Log(ctx)
		if err := r.OverallError(failOnWarns); err != nil {
			merr = append(merr, err)
		}
	}

	if len(merr) != 0 {
		return results, merr
	}
	return results, nil
}
