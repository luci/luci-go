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

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/appstatus"

	"go.chromium.org/luci/cv/internal/common"
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

// LoadRun returns Run from Datastore while checking ACLs.
//
// Errors is tagged with appstatus for all but internal errors.
func LoadRun(ctx context.Context, id common.RunID) (*run.Run, error) {
	// The caller shouldn't be able to distinguish between Run not existing and
	// not having access to the Run, because it may leak the existence of the Run.
	const notFoundMsg = "Run not found"

	r := &run.Run{ID: id}
	switch err := datastore.Get(ctx, r); {
	case err == datastore.ErrNoSuchEntity:
		return nil, appstatus.Error(codes.NotFound, notFoundMsg)
	case err != nil:
		return nil, errors.Annotate(err, "failed to fetch Run").Tag(transient.Tag).Err()
	}

	switch yes, err := CheckRunAccess(ctx, r); {
	case err != nil:
		return nil, err
	case yes:
		return r, nil
	default:
		return nil, appstatus.Error(codes.NotFound, notFoundMsg)
	}
}
