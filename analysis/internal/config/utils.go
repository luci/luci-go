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

package config

import (
	"context"

	"go.chromium.org/luci/common/errors"
)

// This file contains utility methods related to config that do not make
// sense to live in other packages because they are used from multiple places.

// IsProjectEnabledForIngestion returns if the LUCI project is enabled for
// ingestion. By default, all LUCI projects are enabled for ingestion, but
// it is possible to limit ingestion to an allowlisted set in the
// service configuration.
func IsProjectEnabledForIngestion(ctx context.Context, project string) (bool, error) {
	cfg, err := Get(ctx)
	if err != nil {
		return false, errors.Annotate(err, "get service config").Err()
	}
	if !cfg.Ingestion.GetProjectAllowlistEnabled() {
		return true, nil
	}
	allowList := cfg.Ingestion.GetProjectAllowlist()
	for _, entry := range allowList {
		if project == entry {
			return true, nil
		}
	}
	return false, nil
}
