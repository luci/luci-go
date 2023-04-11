// Copyright 2017 The LUCI Authors.
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

package buildbucket

import (
	"context"

	"go.chromium.org/luci/buildbucket/bbperms"
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/realms"
)

// FilterVisibleBuilders returns a list of builders that are visible to the
// current user.
// When project is not an empty string, only builders in the specified project
// will be returned.
func FilterVisibleBuilders(c context.Context, builders []*buildbucketpb.BuilderID, project string) ([]*buildbucketpb.BuilderID, error) {
	filteredBuilders := make([]*buildbucketpb.BuilderID, 0)

	bucketPermissions := make(map[string]bool)

	for _, builder := range builders {
		if project != "" && builder.Project != project {
			continue
		}

		realm := realms.Join(builder.Project, builder.Bucket)

		allowed, ok := bucketPermissions[realm]
		if !ok {
			var err error
			allowed, err = auth.HasPermission(c, bbperms.BuildersList, realm, nil)
			if err != nil {
				return nil, err
			}
			bucketPermissions[realm] = allowed
		}

		if !allowed {
			continue
		}

		filteredBuilders = append(filteredBuilders, builder)
	}

	return filteredBuilders, nil
}
