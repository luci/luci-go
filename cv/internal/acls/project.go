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

	"go.chromium.org/luci/cv/internal/configs/prjcfg"
)

// CheckProjectAccess checks if the calling user has access to the LUCI project.
//
// Returns true if project exists and is active and user has access to this
// LUCI project, false otherwise.
func CheckProjectAccess(ctx context.Context, project string) (bool, error) {
	// TODO(yiwzhang): Consider returning a enum (UNKNOWN, ALLOWED, DENIED,
	// PROJECT_NOT_EXISTS) so that callsite can explicitly specify logic to
	// handle various scenarios.
	//
	// TODO(https://crbug.com/1233963): design & implement & test.

	// Check whether project is active.
	switch m, err := prjcfg.GetLatestMeta(ctx, project); {
	case err != nil:
		return false, err
	case m.Status != prjcfg.StatusEnabled:
		return false, nil
	}

	switch yes, err := checkLegacyCQStatusAccess(ctx, project); {
	case err != nil:
		return false, err
	case yes:
		return true, nil
	}

	// Default to no access.
	return false, nil
}
