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

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/run"
)

// CheckRunAccess checks if the calling user has access to the Run.
//
// Returns true if user has access, false otherwise.
func CheckRunAccess(ctx context.Context, r *run.Run) (bool, error) {
	// TODO(https://crbug.com/1233963): design & implement & test.

	switch yes, err := checkLegacyCQStatusAccess(ctx, r.ID.LUCIProject()); {
	case err != nil:
		return false, err
	case yes:
		return true, nil
	}

	// Default to no access.
	return false, nil
}

// NewRunReadChecker returns an object to check read premission when loading
// Runs.
//
// If current identity lacks read permission, ensures an appopriate appstatus
// package error is returned.
//
// Example:
//   r, err := run.LoadRuns(ctx, id, acls.NewRunReadChecker())
func NewRunReadChecker() runReadChecker { return runReadChecker{} }

// runNotFoundMsg is used as textual reason for gRPC NotFound code.
//
// Rational: the caller shouldn't be able to distinguish between Run not
// existing and not having access to the Run, because it may leak the existence
// of the Run.
const runNotFoundMsg = "Run not found"

// runReadChecker checks read access to Runs.
type runReadChecker struct{}

func (c runReadChecker) Before(ctx context.Context, id common.RunID) error {
	return nil
}
func (c runReadChecker) After(ctx context.Context, r *run.Run) error {
	if r == nil {
		return appstatus.Error(codes.NotFound, runNotFoundMsg)
	}
	switch yes, err := CheckRunAccess(ctx, r); {
	case err != nil:
		return err
	case yes:
		return nil
	default:
		return appstatus.Error(codes.NotFound, runNotFoundMsg)
	}
}
