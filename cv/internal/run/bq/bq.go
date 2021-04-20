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

	"github.com/golang/protobuf/ptypes"
	//"github.com/golang/protobuf/ptypes/timestamp"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"

	cvbqpb "go.chromium.org/luci/cv/api/bigquery/v1"
	cvbq "go.chromium.org/luci/cv/internal/bq"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/migration"
	"go.chromium.org/luci/cv/internal/run"
)

func SendRun(ctx context.Context, id common.RunID) error {
	a, err := makeAttempt(ctx, id)
	if err != nil {
		return errors.Annotate(err, "failed to make Attempt").Err()
	}

	// During the migration period when CQDaemon does most checks and triggers
	// builds, CV can't populate all of the fields of Attempt without the
	// information from CQDaemon; so for finished Attempts reported by
	// CQDaemon, we can fill in the remaining fields.
	cqda, err := fetchCQDAttempt(ctx, id)
	if err != nil {
		logging.WithError(err).Debugf(ctx, "not filling in with Attempt from CQD for Run %s", string(id))
	}
	if cqda != nil {
		a.Builds = cqda.Builds
		a.Substatus = cqda.Substatus
		a.HasCustomRequirement = cqda.HasCustomRequirement
	}

	// The operation ID is used for deduplicating row send attempts, so that row
	// sending is idempotent.
	opID := string(id)
	// When we first start sending rows, we want to send to a separate table name.
	// TODO(crbug.com/1173168): Change the table name when CQD stops sending rows.
	return cvbq.SendRow(ctx, "raw", "attempts_cv", opID, a)
}

func makeAttempt(ctx context.Context, id common.RunID) (*cvbqpb.Attempt, error) {
	r := run.Run{ID: id}
	switch err := datastore.Get(ctx, &r); {
	case err == datastore.ErrNoSuchEntity:
		return nil, errors.Reason("Run not found").Err()
	case err != nil:
		return nil, errors.Annotate(err, "failed to fetch Run").Tag(transient.Tag).Err()
	}

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
	runcls := []*run.RunCL{}
	for _, clid := range r.CLs {
		runcls = append(runcls, &run.RunCL{ID: clid})
	}
	if err := datastore.Get(ctx, runcls); err != nil {
		return nil, errors.Annotate(err, "failed to fetch RunCLs").Err()
	}
	gerritChanges := []*cvbqpb.GerritChange{}
	for _, cl := range runcls {
		gerritChanges = append(gerritChanges, toGerritChange(cl, r))
	}

	startTime, err := ptypes.TimestampProto(r.StartTime)
	if err != nil {
		return nil, errors.Annotate(err, "failed to convert StartTime").Err()
	}
	endTime, err := ptypes.TimestampProto(r.StartTime)
	if err != nil {
		return nil, errors.Annotate(err, "failed to convert EndTime").Err()
	}

	// TODO(crbug.com/1173168): Add a separate CreateTime field to Attempt.
	// Note that StartTime is CL processing start time, which is different
	// from when the Run was created or triggered, see crbug.com/1105669.
	a := &cvbqpb.Attempt{
		Key:                  string(r.ID),
		LuciProject:          r.ID.LUCIProject(),
		ConfigGroup:          r.ConfigGroupID.Name(),
		ClGroupKey:           clGroupKey(runcls),
		EquivalentClGroupKey: equivalentClGroupKey(runcls),
		StartTime:            startTime,
		EndTime:              endTime,
		GerritChanges:        gerritChanges,
		Status:               attemptStatus(r),
		Substatus:            cvbqpb.AttemptSubstatus_NO_SUBSTATUS,
		// Builds, Substatus and HasCustomRequirement are not known to CV yet
		// during the migration state, so they should be filled in with Attempt
		// from CQD if possible.
	}
	return a, nil
}

func toGerritChange(cl *run.RunCL, r run.Run) *cvbqpb.GerritChange {
	// TODO(crbug.com/1173168): Implement.
	// GerritChange has: Host, Project, Change, Patchset,
	// EarliestEquivalentPatchset, TriggerTime, Mode (e.g. dry run, full run),
	// SubmitStatus (PENDING, UNKNOWN, FAILURE, SUCCESS).
	// cl.Detail has: Patchset, MinEquivalentPatchset, ExternalUpdateTime; and
	// cl.Detail.Gerrit has: Host, Info, where info is Gerrit ChangeInfo with
	// number, project, ...
	gc := &cvbqpb.GerritChange{
		Host:                       "",
		Project:                    "",
		Change:                     0,
		Patchset:                   0,
		EarliestEquivalentPatchset: 0,
		TriggerTime:                nil,
		Mode:                       r.Mode.BQAttemptMode(),
	}
	return gc
}

// fetchCQDAttempt fetches an Attempt from CQDaemon if available.
//
// Returns nil if no Attempt is available.
func fetchCQDAttempt(ctx context.Context, id common.RunID) (*cvbqpb.Attempt, error) {
	v := migration.VerifiedCQDRun{ID: id}
	switch err := datastore.Get(ctx, &v); {
	case err == datastore.ErrNoSuchEntity:
		// A Run might end without a VerifiedCQDRun reported if the run is canceled.
		logging.WithError(err).Debugf(ctx, "no VerifiedCQDRun found for Run %s", string(id))
	case err != nil:
		return nil, errors.Annotate(err, "failed to fetch VerifiedCQDRun").Tag(transient.Tag).Err()
	}
	if v.Payload == nil || v.Payload.Run == nil || v.Payload.Run.Attempt == nil {
		logging.Warningf(ctx, "Attempt not found in VerifiedCQDRun for Run %s", string(id))
	}
	return v.Payload.Run.Attempt, nil
}

func attemptStatus(r run.Run) cvbqpb.AttemptStatus {
	// If all CLs are submitted successfully and the run is a full run,
	// then Attempt status should be set to success (see crbug.com/1114686).
	if r.Mode == run.FullRun && len(r.Submission.Cls) == len(r.Submission.SubmittedCls) {
		return cvbqpb.AttemptStatus_SUCCESS
	}
	// Run.Status mostly corresponds to Attempt.Status, except that unfinished
	// runs are broken down into more detail for Run.Status.
	switch r.Status {
	case run.Status_PENDING:
		fallthrough
	case run.Status_RUNNING:
		fallthrough
	case run.Status_WAITING_FOR_SUBMISSION:
		fallthrough
	case run.Status_SUBMITTING:
		return cvbqpb.AttemptStatus_STARTED
	case run.Status_SUCCEEDED:
		return cvbqpb.AttemptStatus_SUCCESS
	case run.Status_FAILED:
		return cvbqpb.AttemptStatus_FAILURE
	case run.Status_CANCELLED:
		return cvbqpb.AttemptStatus_ABORTED
	}
	// Not expected to ever happen.
	return cvbqpb.AttemptStatus_ATTEMPT_STATUS_UNSPECIFIED
}

// clGroupKey constructs an opaque key unique to this set of CLs and patchsets.
func clGroupKey(cls []*run.RunCL) string {
	// TODO(crbug.com/1173168): Implement. In CQDaemon this is based on the
	// sorted CL triples (host, number, patchset).
	// TODO(crbug.com/1090123): This should not be equal with equivalentClGroupKey.
	return ""
}

// equivalentClGroupKey constructs an opaque key unique to a set of CLs and
// their min equivalent patchsets.
func equivalentClGroupKey(cls []*run.RunCL) string {
	// TODO(crbug.com/1173168): Implement. In CQDaemon this is based on the
	// sorted CL triples (hostname, number, min equivalent patchset).
	// TODO(crbug.com/1090123): This should not be equal with CL group key.
	return ""
}
