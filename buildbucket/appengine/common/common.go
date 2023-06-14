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
// A shortcut for GetBuildEntities with only Build.
func GetBuild(ctx context.Context, id int64) (*model.Build, error) {
	entities, err := GetBuildEntities(ctx, id, model.BuildKind)
	if err != nil {
		return nil, err
	}
	return entities[0].(*model.Build), nil
}

func GetBuildEntities(ctx context.Context, id int64, kinds ...string) ([]any, error) {
	if len(kinds) == 0 {
		return nil, errors.Reason("no entities to get").Err()
	}

	var toGet []any
	bk := datastore.KeyForObj(ctx, &model.Build{ID: id})
	appendEntity := func(kind string) error {
		switch kind {
		case model.BuildKind:
			toGet = append(toGet, &model.Build{ID: id})
		case model.BuildStatusKind:
			toGet = append(toGet, &model.BuildStatus{Build: bk})
		case model.BuildStepsKind:
			toGet = append(toGet, &model.BuildSteps{Build: bk})
		case model.BuildInfraKind:
			toGet = append(toGet, &model.BuildInfra{Build: bk})
		case model.BuildInputPropertiesKind:
			toGet = append(toGet, &model.BuildInputProperties{Build: bk})
		case model.BuildOutputPropertiesKind:
			toGet = append(toGet, &model.BuildOutputProperties{Build: bk})
		default:
			return errors.Reason("unknown entity kind %s", kind).Err()
		}
		return nil
	}

	for _, kind := range kinds {
		if err := appendEntity(kind); err != nil {
			return nil, err
		}
	}

	switch err := datastore.Get(ctx, toGet...); {
	case errors.Contains(err, datastore.ErrNoSuchEntity):
		return nil, perm.NotFoundErr(ctx)
	case err != nil:
		return nil, errors.Annotate(err, "error fetching build entities with ID %d", id).Err()
	default:
		return toGet, nil
	}
}
