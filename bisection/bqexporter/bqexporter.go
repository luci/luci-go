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

// Package bqexporter handles export to BigQuery.
package bqexporter

import (
	"context"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/gae/service/info"

	"go.chromium.org/luci/bisection/model"
	bqpb "go.chromium.org/luci/bisection/proto/bq"
	pb "go.chromium.org/luci/bisection/proto/v1"
	"go.chromium.org/luci/bisection/util/bqutil"
	"go.chromium.org/luci/bisection/util/datastoreutil"
)

// The number of days to look back for past analyses.
// We only look back and export analyses within the past 14 days.
const daysToLookBack = 14

// ExportTestAnalyses exports test failure analyses to BigQuery.
// A test failure analysis will be exported if it satisfies the following conditions:
//  1. It has not been exported yet.
//  2. It was created within the past 14 days.
//  3. Has ended.
//  4. If it found culprit, then actions must have been taken.
//
// The limit of 14 days is chosen to save the query time. It is also because if the exporter
// is broken for some reasons, we will have 14 days to fix it.
func ExportTestAnalyses(ctx context.Context) error {
	// TODO (nqmtuan): We should read it from config.
	// But currently we only have per-project config, not service config.
	// So for now we are hard-coding it.
	if !isEnabled(ctx) {
		logging.Warningf(ctx, "export test analyses is not enabled")
	}

	client, err := NewClient(ctx, info.AppID(ctx))
	if err != nil {
		return errors.Fmt("new client: %w", err)
	}
	defer client.Close()
	err = export(ctx, client)
	if err != nil {
		return errors.Fmt("export: %w", err)
	}
	return nil
}

type ExportClient interface {
	EnsureSchema(ctx context.Context) error
	Insert(ctx context.Context, rows []*bqpb.TestAnalysisRow) error
	ReadTestFailureAnalysisRows(ctx context.Context) ([]*TestFailureAnalysisRow, error)
}

func export(ctx context.Context, client ExportClient) error {
	err := client.EnsureSchema(ctx)
	if err != nil {
		return errors.Fmt("ensure schema: %w", err)
	}

	analyses, err := fetchTestAnalyses(ctx)
	if err != nil {
		return errors.Fmt("fetch test analyses: %w", err)
	}
	logging.Infof(ctx, "There are %d test analyses fetched from datastore", len(analyses))

	// Read existing rows from bigquery.
	bqrows, err := client.ReadTestFailureAnalysisRows(ctx)
	if err != nil {
		return errors.Fmt("read test failure analysis rows: %w", err)
	}
	logging.Infof(ctx, "There are %d existing rows in BigQuery", len(bqrows))

	// Filter out existing rows.
	// Construct a map for fast filtering.
	existingIDs := map[int64]bool{}
	for _, r := range bqrows {
		existingIDs[r.AnalysisID] = true
	}

	// Construct BQ rows.
	rowsToInsert := []*bqpb.TestAnalysisRow{}
	for _, tfa := range analyses {
		if _, ok := existingIDs[tfa.ID]; !ok {
			row, err := bqutil.TestFailureAnalysisToBqRow(ctx, tfa)
			if err != nil {
				return errors.Fmt("test failure analysis to bq row for analysis ID: %d: %w", tfa.ID, err)
			}
			rowsToInsert = append(rowsToInsert, row)
		}
	}
	logging.Infof(ctx, "After filtering, there are %d rows to insert to BigQuery.", len(rowsToInsert))

	// Insert into BQ.
	err = client.Insert(ctx, rowsToInsert)
	if err != nil {
		return errors.Fmt("insert: %w", err)
	}
	return nil
}

// fetchTestAnalyses returns the test analyses that:
// - Created within 14 days
// - Has ended
// - If it found a culprit, then either the actions have been taken,
// or the it has ended more than 1 day ago.
func fetchTestAnalyses(ctx context.Context) ([]*model.TestFailureAnalysis, error) {
	// Query all analyses within 14 days.
	cutoffTime := clock.Now(ctx).Add(-time.Hour * 24 * daysToLookBack)
	q := datastore.NewQuery("TestFailureAnalysis").Gt("create_time", cutoffTime).Order("-create_time")
	analyses := []*model.TestFailureAnalysis{}
	err := datastore.GetAll(ctx, q, &analyses)
	if err != nil {
		return nil, errors.Fmt("get test analyses: %w", err)
	}

	// Check that the analyses ended and actions were taken.
	results := []*model.TestFailureAnalysis{}
	for _, tfa := range analyses {
		// Ignore all analyses that have not ended.
		if !tfa.HasEnded() {
			continue
		}
		// If the analyses did not find any culprit, then we don't
		// need to check for culprit actions.
		if tfa.Status != pb.AnalysisStatus_FOUND {
			results = append(results, tfa)
			continue
		}

		//Get culprit.
		culprit, err := datastoreutil.GetVerifiedCulpritForTestAnalysis(ctx, tfa)
		if err != nil {
			return nil, errors.Fmt("get verified culprit: %w", err)
		}
		if culprit == nil {
			return nil, errors.Fmt("no culprit found for analysis %d", tfa.ID)
		}

		// Make an exception: If an analysis ended more than 1 day ago, and
		// HasTakenActions is still set to false, most likely something was stuck
		// that prevent the filed from being set. In this case, we want to
		// export the analysis anyway, since there will be no changes to it.
		// It also let us export the analyses without suspect's HasTakenActions field set.
		oneDayAgo := clock.Now(ctx).Add(-time.Hour * 24)
		if !culprit.HasTakenActions && tfa.EndTime.Before(oneDayAgo) {
			// Logging for visibility.
			logging.Warningf(ctx, "Analysis %d has ended more than a day ago, but actions are not taken", tfa.ID)
		}

		if culprit.HasTakenActions || tfa.EndTime.Before(oneDayAgo) {
			results = append(results, tfa)
		}
	}
	return results, nil
}

func isEnabled(ctx context.Context) bool {
	// return info.AppID(ctx) == "luci-bisection-dev"
	return true
}
