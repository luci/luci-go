// Copyright 2023 The LUCI Authors.
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

package common

import (
	"context"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/buildbucket/appengine/internal/perm"
	"go.chromium.org/luci/buildbucket/appengine/model"
)

// GetBuild returns the build with the given ID or NotFound appstatus if it is
// not found.
func GetBuild(ctx context.Context, id int64) (*model.Build, error) {
	bld := &model.Build{ID: id}
	switch err := datastore.Get(ctx, bld); {
	case err == datastore.ErrNoSuchEntity:
		return nil, perm.NotFoundErr(ctx)
	case err != nil:
		return nil, errors.Annotate(err, "error fetching build with ID %d", id).Err()
	default:
		return bld, nil
	}
}
