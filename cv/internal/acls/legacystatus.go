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

package acls

import (
	"context"
	"time"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/caching/layered"

	"go.chromium.org/luci/cv/internal/configs/prjcfg"
	"go.chromium.org/luci/cv/internal/configs/validation"
)

const (
	cqStatusInternalCrIAGroup = "internal-cq-status-access"
	legacyCQStatusHostTTL     = 20 * time.Minute
)

// legacyCQStatusHostCache caches CQ status hosts per LUCI project.
var legacyCQStatusHostCache = layered.RegisterCache(layered.Parameters[string]{
	ProcessCacheCapacity: 100,
	GlobalNamespace:      "acls_legacy_cqstatus_v1",
	Marshal: func(host string) ([]byte, error) {
		return []byte(host), nil
	},
	Unmarshal: func(blob []byte) (string, error) {
		return string(blob), nil
	},
})

// checkLegacyCQStatusAccess checks if the calling user has access to the Runs
// of the given LUCI project via the legacy CQ status app.
//
// Returns true if user has access.
//
// Each LUCI project can configure cq status app usage in 3 diff ways:
//   - (P) public via "chromium-cq-status.appspot.com"
//   - (I) internal via "internal-cq-stauts.appspot.com"
//   - (N) none
//
// Thus, the project config can be used to infer visibility of project's Runs.
//
// See also https://crbug.com/1250737.
// TODO(crbug/1233963): remove this legacy after implementing CV ACLs.
func checkLegacyCQStatusAccess(ctx context.Context, luciProject string) (bool, error) {
	switch host, err := loadCQStatusHost(ctx, luciProject); {
	case err != nil:
		return false, err
	case host == validation.CQStatusHostPublic:
		return true, nil
	case host == validation.CQStatusHostInternal:
		return auth.IsMember(ctx, cqStatusInternalCrIAGroup)
	case host == "":
		return false, nil
	default:
		logging.Errorf(ctx, "crbug/1250737: Unrecognized CQ Status Host %q", host)
		return false, nil
	}
}

// loadCQStatusHost returns CQ status host configured for a LUCI projects.
func loadCQStatusHost(ctx context.Context, luciProject string) (string, error) {
	return legacyCQStatusHostCache.GetOrCreate(ctx, luciProject, func() (string, time.Duration, error) {
		m, err := prjcfg.GetLatestMeta(ctx, luciProject)
		switch {
		case err != nil:
			return "", 0, err
		case m.Status != prjcfg.StatusEnabled:
			// Cache for disabled/deleted projects, too.
			return "", legacyCQStatusHostTTL, nil
		}
		// All ConfigGroups have the same CQStatusHost, so just load the first one.
		switch cfg, err := prjcfg.GetConfigGroup(ctx, m.Project, m.ConfigGroupIDs[0]); {
		case err != nil:
			return "", 0, err
		default:
			return cfg.CQStatusHost, legacyCQStatusHostTTL, nil
		}
	})
}
