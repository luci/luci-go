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
	cvbq "go.chromium.org/luci/cv/internal/bq"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/migration"
	"go.chromium.org/luci/cv/internal/run"
)

func SendRun(ctx context.Context, id common.RunID) error {
	a, err := fetchCQDAttempt(ctx, id)
	if err != nil {
		return err
	}
	// opID is used for deduplicating row send attempts. If a row for one run
	// is accidentally sent twice, we don't want to add duplicate rows.
	opID := string(id)
	return cvbq.SendRow(ctx, "raw", "attempts_new", opID, a)
}

// fetchCQDAttempt fetches an Attempt and prepares it to be sent to BQ.
//
// This function uses VerifiedCQDRun, using an Attempt that was reported by CQDaemon,
// is used only during migration, and should be removed and replaced
// after migration from CQDaemon is complete.
func fetchCQDAttempt(ctx context.Context, id common.RunID) (*cvbqpb.Attempt, error) {
	// First fetch the Attempt proto as reported by CQDaemon.
	v := migration.VerifiedCQDRun{ID: id}
	switch err := datastore.Get(ctx, &v); {
	case err == datastore.ErrNoSuchEntity:
		// TODO:
		// This case is not necessarily an error. A Run could end without
		// VerifiedCQDRun reported. For example, if a new patchset is uploaded
		// while tryjob is still running. CV will bring Run to final state and
		// cancels the verification CQDaemon is running.
		return nil, errors.Reason("VerifiedCQDRun not found").Err()
	case err != nil:
		return nil, errors.Annotate(err, "failed to fetch VerifiedCQDRun").Tag(transient.Tag).Err()
	}
	if v.Payload == nil || v.Payload.Run == nil || v.Payload.Run.Attempt == nil {
		return nil, errors.Reason("Attempt not found in VerifiedCQDRun").Err()
	}
	a := v.Payload.Run.Attempt

	// Fetch the Run which contains the record of Submission.
	r := run.Run{ID: id}
	switch err := datastore.Get(ctx, &r); {
	case err == datastore.ErrNoSuchEntity:
		return nil, errors.Reason("Run not found").Err()
	case err != nil:
		return nil, errors.Annotate(err, "failed to fetch Run").Tag(transient.Tag).Err()
	}
	s := r.Submission

	// XXX INCOMPLETE
	// In order to look up the CL (host, project, change number) of the CL, we
	// need to fetch the RunCL entities. TODO: iterate through CLs, fetching
	// RunCLs from datastore; for each, get the Details (snapshot) which has
	// the required details to look up CL. the Submission object contains a
	// list of CLIDs for CLs that were submitted so for each of those we'll
	// want to mark the Attempt.GerritChange has submitted and for all others,
	// mark it as not submitted.
	// https://source.chromium.org/chromium/infra/infra/+/master:go/src/go.chromium.org/luci/cv/internal/changelist/storage.proto
	// https://source.chromium.org/chromium/infra/infra/+/master:go/src/go.chromium.org/luci/cv/api/bigquery/v1/attempt.proto
	// https://source.chromium.org/chromium/infra/infra/+/master:go/src/go.chromium.org/luci/cv/internal/run/model.go
	//for _, cl := range r.Cls {
	// This will be used to set SubmitStatus in entries in Attempt.GerritChanges
	//
	// To set Status and maybe Substatus in Attempt, we can probably use Run.Status.
	return a, nil
}
