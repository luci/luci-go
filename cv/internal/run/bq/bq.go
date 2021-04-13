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

// Package bq is responsible for preparing rows to send to BigQuery upon
// completion of a Run.

package bq

import (
	"context"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"

	cvbqpb "go.chromium.org/luci/cv/api/bigquery/v1"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/migration"
	"go.chromium.org/luci/cv/internal/run"
)

func SendRun(ctx context.Context, id common.RunID) error {
	//Load Run entity; load Run entity.
	v := run.Run{ID: id}
	err := datastore.Get(ctx, &v)
	if err == datastore.ErrNoSuchEntity {
		return nil, errors.Annotate(err, "no Run found trying to get Attempt").Err()
	} else if err != nil {
		return nil, errors.Annotate(err, "failed to fetch run").Tag(transient.Tag).Err()
	}

	return nil
}

// fetchCQDAttempt fetches an Attempt and prepares it to be sent to BQ.
//
// This function uses VerifiedCQDRun, using an Attempt that was reported by CQDaemon,
// is used only during migration, and should be removed and replaced
// after migration from CQDaemon is complete.
func fetchCQDAttempt(ctx context.Context, id common.RunID) (*cvbqpb.Attempt, error) {
	// First fetch the Attempt proto as reported by CQDaemon.
	v := migration.VerifiedCQDRun{ID: id}
	err := datastore.Get(ctx, &v)
	if err == datastore.ErrNoSuchEntity {
		return nil, errors.Annotate(err, "no VerifiedCQDRun found trying to get Attempt").Err()
	} else if err != nil {
		return nil, errors.Annotate(err, "failed to fetch VerifiedCQDRun").Tag(transient.Tag).Err()
	}
	if v.Payload == nil || v.Payload.Run == nil || v.Payload.Run.Attempt == nil {
		return nil, errors.Annotate(err, "Attempt not found in VerifiedCQDRun").Err()
	}
	a := v.Payload.Run.Attempt

	// Fetch the Run which contains the record of Submission.
	r := run.Run{ID: id}
	err := datastore.Get(ctx, &r)
	if err == datastore.ErrNoSuchEntity {
		return nil, errors.Annotate(err, "no Run found trying to get Attempt").Err()
	} else if err != nil {
		return nil, errors.Annotate(err, "failed to fetch Run").Tag(transient.Tag).Err()
	}

	s := r.Submission
	// Set the following things: SubmitStatus in entries in Attempt.GerritChanges
	// and Status, Substats in Attempt.
	// Set the final submit status in Attempt, which should be set in Run.
	// INCOMPLETE
	return a, nil
}
