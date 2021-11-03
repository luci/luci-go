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

	"go.chromium.org/luci/grpc/appstatus"

	"go.chromium.org/luci/cv/internal/configs/prjcfg"
)

// CheckProjectAccess checks if the calling user has access to the LUCI project.
//
// Returns true if user has access, false otherwise.
func CheckProjectAccess(ctx context.Context, project string) (bool, error) {
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

// projectNotFoundMsg is used as textual reason for gRPC NotFound code.
//
// Rational: the caller shouldn't be able to distinguish between project not
// existing and not having access to the project, because it may leak the
// existence of the project.
const projectNotFoundMsg = "Project not found"

// grpcProjectAccess checks if user has access to the project.
//
// Returns nil if so or an appstatus-tagged error if not.
func grpcCheckProjectAccess(ctx context.Context, project string) error {
	switch yes, err := CheckProjectAccess(ctx, project); {
	case err != nil:
		return err
	case yes:
		return nil
	default:
		return appstatus.Error(codes.NotFound, projectNotFoundMsg)
	}
}
