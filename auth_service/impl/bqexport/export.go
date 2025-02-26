// Copyright 2025 The LUCI Authors.
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

// Package bqexport contains functionality related to exporting the
// authorization data from LUCI Auth Service to BigQuery (BQ).
package bqexport

import (
	"context"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"

	"go.chromium.org/luci/auth_service/api/bqpb"
	"go.chromium.org/luci/auth_service/impl/model"
	"go.chromium.org/luci/auth_service/impl/model/graph"
)

func CronHandler(ctx context.Context) error {
	logging.Infof(ctx, "attempting BQ export")

	err := Run(ctx)
	if err != nil {
		if transient.Tag.In(err) {
			// Return the error to signal retry.
			return err
		}

		// Error is non-transient; log the error but do not retry.
		err = errors.Annotate(err, "failed BQ export").Err()
		logging.Errorf(ctx, err.Error())
	}

	return nil
}

// Run exports the authorization data from the latest AuthDB snapshot to BQ.
func Run(ctx context.Context) error {
	start := timestamppb.New(clock.Now(ctx))
	latest, err := model.GetAuthDBSnapshotLatest(ctx)
	if err != nil {
		return errors.Annotate(err, "failed to get latest snapshot").Err()
	}

	authDB, err := model.GetAuthDBFromSnapshot(ctx, latest.AuthDBRev)
	if err != nil {
		return errors.Annotate(err, "failed to parse AuthDB from latest snapshot").Err()
	}

	groups, err := expandGroups(ctx, authDB)
	if err != nil {
		return errors.Annotate(err, "failed to expand all groups").Err()
	}

	return doExport(ctx, latest.AuthDBRev, start, groups)
}

func doExport(ctx context.Context, authDBRev int64, ts *timestamppb.Timestamp,
	groups []*graph.ExpandedGroup) (reterr error) {
	groupRows := make([]*bqpb.GroupRow, len(groups))
	for i, group := range groups {
		groupRows[i] = toGroupRow(group, authDBRev, ts)
	}

	client, err := NewClient(ctx)
	if err != nil {
		return err
	}
	defer func() {
		err := client.Close()
		if reterr == nil {
			reterr = errors.Annotate(err, "failed to close BQ client").Err()
		}
	}()

	// Insert all groups.
	if err := client.InsertGroups(ctx, groupRows); err != nil {
		return errors.Annotate(err,
			"failed to insert all groups for AuthDB rev %d at %s",
			authDBRev, ts.String()).Err()
	}

	return nil
}

func toGroupRow(group *graph.ExpandedGroup,
	authDBRev int64, ts *timestamppb.Timestamp) *bqpb.GroupRow {
	return &bqpb.GroupRow{
		Name:        group.Name,
		Description: group.Description,
		Owners:      group.Owners,
		Members:     group.Members.ToSortedSlice(),
		Globs:       group.Globs.ToSortedSlice(),
		Subgroups:   group.Nested.ToSortedSlice(),
		AuthdbRev:   authDBRev,
		ExportedAt:  ts,
	}
}
