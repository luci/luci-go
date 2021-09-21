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

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/auth"

	"go.chromium.org/luci/cv/internal/configs/prjcfg"
	"go.chromium.org/luci/cv/internal/run"
)

const (
	cqStatusHostPublic        = "chromium-cq-status.appspot.com"
	cqStatusHostInternal      = "internal-cq-status.appspot.com"
	cqStatusInternalCrIAGroup = "googlers"
)

// CheckLegacyCQStatusAccess checks if the calling user has access to the Run
// via legacy CQ status app.
//
// Returns true if user has access.
//
// Each LUCI project can configure cq status app usage in 3 diff ways:
//  * (P) public via "chromium-cq-status.appspot.com"
//  * (I) internal via "internal-cq-stauts.appspot.com"
//  * (N) none
//
// Thus, the project config can be used to infer visibility of project's Runs.
//
// See also https://crbug.com/1250737.
// TODO(crbug/1233963): remove this legacy after implementing CV ACLs.
func CheckLegacyCQStatusAccess(ctx context.Context, r *run.Run) (bool, error) {
	// Although Run itself is associated with a ConfigGroup, it may be pinned at
	// very old revision, so prefer the newest config.
	m, err := prjcfg.GetLatestMeta(ctx, r.ID.LUCIProject())
	switch {
	case err != nil:
		return false, err
	case m.Status != prjcfg.StatusEnabled:
		return false, nil
	}
	// All ConfigGroups have the same CQStatusHost, so just load the first one.
	switch cfg, err := prjcfg.GetConfigGroup(ctx, m.Project, m.ConfigGroupIDs[0]); {
	case err != nil:
		return false, err
	case cfg.CQStatusHost == cqStatusHostPublic:
		return true, nil
	case cfg.CQStatusHost == cqStatusHostInternal:
		return auth.IsMember(ctx, cqStatusInternalCrIAGroup)
	case cfg.CQStatusHost == "":
		return false, nil
	default:
		logging.Errorf(ctx, "crbug/1250737: Unrecognized CQ Status Host %q", cfg.CQStatusHost)
		return false, nil
	}
}
