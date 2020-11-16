// Copyright 2020 The LUCI Authors.
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

// Package updater fetches latest CL data from Gerrit.
package updater

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcutil"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/gerrit"
)

// UpdateCL fetches latest info from Gerrit.
//
// If datastore already contains snapshot with Gerrit-reported update time equal
// to or after updatedHint, then no updating or querying will be performed.
// To force an update, provide as time.Time{} as updatedHint.
func UpdateCL(ctx context.Context, luciProject, host string, change int64, updatedHint time.Time) error {
	fetcher := fetcher{
		luciProject: luciProject,
		host:        host,
		change:      change,
	}
	externalID, err := changelist.GobID(host, change)
	if err != nil {
		return err
	}
	var clid changelist.CLID
	if !updatedHint.IsZero() {
		switch cl, err := externalID.Get(ctx); {
		case err == nil:
			t := cl.Snapshot.GetExternalUpdateTime().AsTime()
			if !t.IsZero() && updatedHint.Before(t) {
				logging.Debugf(ctx, "Updating %s to %s skipped, already at %s", externalID, updatedHint, t)
				return nil
			}
			fetcher.prior = cl.Snapshot
			clid = cl.ID
		case err != datastore.ErrNoSuchEntity:
			return err
		}
	}

	fetcher.g, err = gerrit.CurrentClient(ctx, host, luciProject)
	if err != nil {
		return err
	}

	snapshot, err := fetcher.do(ctx)
	if err != nil {
		return err
	}

	updated := snapshot.ExternalUpdateTime.AsTime()
	if !updatedHint.IsZero() && updated.Before(updatedHint) {
		logging.Errorf(ctx, "Fetched last Gerrit update of %s, but %s expected", updated, updatedHint)
		return errors.Reason("Fetched stale Gerrit data").Err()
	}

	return changelist.UpdateSnapshot(ctx, externalID, clid, snapshot)
}

// fetcher efficiently computes new snapshot by fetching data from Gerrit.
//
// It ensures each dependency is resolved to an existing CLID,
// creating CLs in datastore as needed. Schedules tasks to update
// dependencies but doesn't wait for them to complete.
//
// prior Snapshot, if given, can reduce RPCs made to Gerrit.
type fetcher struct {
	g           gerrit.CLReaderClient
	luciProject string
	host        string
	change      int64
	prior       *changelist.Snapshot

	result *changelist.Snapshot
}

func (f *fetcher) do(ctx context.Context) (*changelist.Snapshot, error) {
	f.result = &changelist.Snapshot{Kind: &changelist.Snapshot_Gerrit{Gerrit: &changelist.Gerrit{}}}
	// TODO(tandrii): optimize for existing CL case.
	if err := f.new(ctx); err != nil {
		return nil, err
	}
	min, cur, err := gerrit.EquivalentPatchsetRange(f.result.GetGerrit().Info)
	if err != nil {
		return nil, err
	}
	f.result.MinEquivalentPatchset = int32(min)
	f.result.Patchset = int32(cur)
	return f.result, nil
}

// do efficiently fetches new snapshot from Gerrit.
func (f *fetcher) new(ctx context.Context) error {
	req := &gerritpb.GetChangeRequest{
		Number:  f.change,
		Project: f.gerritProjectIfKnown(),
		Options: []gerritpb.QueryOption{
			// These are expensive to compute for Gerrit,
			// CV should not do this needlessly.
			gerritpb.QueryOption_ALL_REVISIONS,
			gerritpb.QueryOption_CURRENT_COMMIT,
			gerritpb.QueryOption_DETAILED_LABELS,
			gerritpb.QueryOption_DETAILED_ACCOUNTS,
			gerritpb.QueryOption_MESSAGES,
			gerritpb.QueryOption_SUBMITTABLE,
			// avoid asking Gerrit to perform expensive operation.
			gerritpb.QueryOption_SKIP_MERGEABLE,
		},
	}
	_, err := f.g.GetChange(ctx, req)
	switch grpcutil.Code(err) {
	case codes.OK:
		// Happy case.
	case codes.NotFound:
		// Either no access OR CL was deleted.
		return errors.New("not implemented")
	case codes.PermissionDenied:
		return errors.New("not implemented")
	default:
		return unhandledError(ctx, err, "failed to fetch %s/%d", f.host, f.change)
	}
	panic("implement")
}

func (f *fetcher) gerritProjectIfKnown() string {
	// Use GetXXX to avoid tripping on nil pointers.
	if project := f.prior.GetGerrit().GetInfo().GetProject(); project != "" {
		return project
	}
	if project := f.result.GetGerrit().GetInfo().GetProject(); project != "" {
		return project
	}
	return ""
}

// unhandledError is used to process and annotate Gerrit errors
func unhandledError(ctx context.Context, err error, format string, args ...interface{}) error {
	msg := fmt.Sprintf(format, args...)
	ann := errors.Annotate(err, msg)
	switch code := grpcutil.Code(err); code {
	case
		codes.OK,
		codes.PermissionDenied,
		codes.NotFound,
		codes.FailedPrecondition:
		// These must be handled before.
		logging.Errorf(ctx, "FIXME unhandled Gerrit error: %s while %s", err, msg)
		return ann.Err()

	case
		codes.InvalidArgument,
		codes.Unauthenticated:
		// This must not happen in practice unless there is a bug in CV or Gerrit.
		logging.Errorf(ctx, "FIXME bug in CV: %s while %s", err, msg)
		return ann.Err()

	case codes.Unimplemented:
		// This shouldn't happen in production, but may happen in development
		// if gerrit.NewRESTClient doesn't actually implement fully the option
		// or entire method that CV is coded to work with.
		logging.Errorf(ctx, "FIXME likely bug in CV: %s while %s", err, msg)
		return ann.Err()

	default:
		// Assume transient. If this turns out non-transient, then its code must be
		// handled explicitly above.
		return ann.Tag(transient.Tag).Err()
	}
}
