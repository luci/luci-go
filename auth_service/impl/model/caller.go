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

package model

import (
	"context"
	"strings"

	"go.chromium.org/luci/server/auth"
)

// canCallerModify returns whether the current identity can modify the given
// group.
func canCallerModify(ctx context.Context, group *AuthGroup) (bool, error) {
	if IsExternalAuthGroupName(group.ID) {
		return false, nil
	}

	return auth.IsMember(ctx, AdminGroup, group.Owners)
}

// canCallerViewMembers returns whether the current identity can view the
// members of the given group.
func canCallerViewMembers(ctx context.Context, group *AuthGroup) (bool, error) {
	// Don't filter if the group is internal.
	if !IsExternalAuthGroupName(group.ID) {
		return true, nil
	}

	// Currently, the Members filter is only necessary for external Google
	// groups.
	if !strings.HasPrefix(group.ID, "google/") {
		return true, nil
	}

	return auth.IsMember(ctx, AdminGroup)
}
