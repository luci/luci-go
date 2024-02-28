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

package util

import (
	"context"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcutil"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/common/lease"
	"go.chromium.org/luci/cv/internal/gerrit"
	"go.chromium.org/luci/cv/internal/run"
)

// StaleCLAgeThreshold is the window that CL entity in datastore should be
// considered latest if refreshed within the threshold.
//
// Large values increase the chance of returning false result on stale Change
// info while low values increase load on Gerrit.
const StaleCLAgeThreshold = 10 * time.Second

// ActionTakeEvalFn is the function signature to evaluate whether certain
// action has already been taken on the given Gerrit Change.
//
// Returns the time when action is taken. Otherwise, returns zero time.
type ActionTakeEvalFn func(rcl *run.RunCL, ci *gerritpb.ChangeInfo) time.Time

// IsActionTakenOnGerritCL checks whether an action specified by `evalFn` has
// been take on a Gerrit CL.
//
// Checks against CV's own cache (CL entity in Datastore) first. If the action
// is not taken and cache is too old (before `now-StaleCLAgeThreshold“),
// then fetch the latest change info from Gerrit and check.
func IsActionTakenOnGerritCL(ctx context.Context, gf gerrit.Factory, rcl *run.RunCL, gerritQueryOpts []gerritpb.QueryOption, evalFn ActionTakeEvalFn) (time.Time, error) {
	cl := changelist.CL{ID: rcl.ID}
	switch err := datastore.Get(ctx, &cl); {
	case err == datastore.ErrNoSuchEntity:
		return time.Time{}, errors.Annotate(err, "CL no longer exists").Err()
	case err != nil:
		return time.Time{}, errors.Annotate(err, "failed to load CL").Tag(transient.Tag).Err()
	}

	switch actionTime := evalFn(rcl, cl.Snapshot.GetGerrit().GetInfo()); {
	case !actionTime.IsZero():
		return actionTime, nil
	case clock.Since(ctx, cl.Snapshot.GetExternalUpdateTime().AsTime()) < StaleCLAgeThreshold:
		// Accept possibility of duplicate messages within the staleCLAgeThreshold.
		return time.Time{}, nil
	}

	// Fetch the latest CL details from Gerrit.
	luciProject := common.RunID(rcl.Run.StringID()).LUCIProject()
	gc, err := gf.MakeClient(ctx, rcl.Detail.GetGerrit().GetHost(), luciProject)
	if err != nil {
		return time.Time{}, err
	}

	req := &gerritpb.GetChangeRequest{
		Project: rcl.Detail.GetGerrit().GetInfo().GetProject(),
		Number:  rcl.Detail.GetGerrit().GetInfo().GetNumber(),
		Options: gerritQueryOpts,
	}
	var ci *gerritpb.ChangeInfo
	outerErr := gf.MakeMirrorIterator(ctx).RetryIfStale(func(opt grpc.CallOption) error {
		ci, err = gc.GetChange(ctx, req, opt)
		switch grpcutil.Code(err) {
		case codes.OK:
			return nil
		case codes.PermissionDenied:
			// This is permanent error which shouldn't be retried.
			return err
		case codes.NotFound:
			return gerrit.ErrStaleData
		default:
			err = gerrit.UnhandledError(ctx, err, "Gerrit.GetChange")
			return err
		}
	})
	switch {
	case err != nil:
		return time.Time{}, errors.Annotate(err, "failed to get the latest Gerrit ChangeInfo").Err()
	case outerErr != nil:
		// Shouldn't happen, unless Mirror iterate itself errors out for some
		// reason.
		return time.Time{}, outerErr
	default:
		return evalFn(rcl, ci), nil
	}
}

// MutateGerritCL calls SetReview on the given Gerrit CL.
//
// Uses mirror iterator and leases the CL before making the Gerrit call.
func MutateGerritCL(ctx context.Context, gf gerrit.Factory, rcl *run.RunCL, req *gerritpb.SetReviewRequest, leaseDuration time.Duration, motivation string) error {
	luciProject := common.RunID(rcl.Run.StringID()).LUCIProject()
	gc, err := gf.MakeClient(ctx, rcl.Detail.GetGerrit().GetHost(), luciProject)
	if err != nil {
		return err
	}

	ctx, cancelLease, err := lease.ApplyOnCL(ctx, rcl.ID, leaseDuration, motivation)
	if err != nil {
		return err
	}
	defer cancelLease()

	outerErr := gf.MakeMirrorIterator(ctx).RetryIfStale(func(opt grpc.CallOption) error {
		_, err = gc.SetReview(ctx, req, opt)
		switch grpcutil.Code(err) {
		case codes.OK:
			return nil
		case codes.PermissionDenied:
			// This is a permanent error which shouldn't be retried.
			return err
		case codes.NotFound:
			// This is known to happen on new CLs or on recently created revisions.
			return gerrit.ErrStaleData
		case codes.FailedPrecondition:
			// SetReview() returns FailedPrecondition, if the CL is abandoned.
			return err
		default:
			err = gerrit.UnhandledError(ctx, err, "Gerrit.SetReview")
			return err
		}
	})
	switch {
	case err != nil:
		return errors.Annotate(err, "failed to call Gerrit.SetReview").Err()
	case outerErr != nil:
		// Shouldn't happen, unless MirrorIterator itself errors out for some
		// reason.
		return outerErr
	default:
		return nil
	}
}
