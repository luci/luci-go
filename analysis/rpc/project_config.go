// Copyright 2022 The LUCI Authors.
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
	"time"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/analysis/internal/config/compiledcfg"
)

// readProjectConfig reads project config. This is intended for use in
// top-level RPC handlers. The caller should directly return any errors
// returned as the error of the RPC; the returned errors have been
// properly annotated with an appstatus.
func readProjectConfig(ctx context.Context, project string) (*compiledcfg.ProjectConfig, error) {
	cfg, err := compiledcfg.Project(ctx, project, time.Time{})
	if err != nil {
		// GRPCifyAndLog will log this, and report an internal error to the caller.
		return nil, errors.Fmt("obtain project config: %w", err)
	}
	return cfg, nil
}
