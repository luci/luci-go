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

	"go.chromium.org/luci/cv/internal/run"
)

// CheckRunAccess checks if the calling user has access to the Run.
//
// Returns true if user has access, false otherwise.
func CheckRunAccess(ctx context.Context, r *run.Run) (bool, error) {
	// TODO(https://crbug.com/1233963): design & implement & test.

	switch yes, err := checkLegacyCQStatusAccess(ctx, r); {
	case err != nil:
		return false, err
	case yes:
		return true, nil
	}

	// Default to no access.
	return false, nil
}
