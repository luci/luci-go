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

package bq

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"

	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"

	cvbqpb "go.chromium.org/luci/cv/api/bigquery/v1"
	"go.chromium.org/luci/cv/internal/common"
	cvbq "go.chromium.org/luci/cv/internal/common/bq"
	"go.chromium.org/luci/cv/internal/migration"
	"go.chromium.org/luci/cv/internal/run"
)

const (
	// CV's own dataset/table.
	CVDataset = "raw"
	CVTable   = "attempts_cv"

	// Legacy CQ dataset.
	legacyProject    = "commit-queue"
	legacyProjectDev = "commit-queue-dev"
	legacyDataset    = "raw"
	legacyTable      = "attempts"
)

func send(ctx context.Context, env *common.Env, client cvbq.Client, id common.RunID) error {
	r := run.Run{ID: id}
	switch err := datastore.Get(ctx, &r); {
	case err == datastore.ErrNoSuchEntity:
		return errors.Reason("Run not found").Err()
	case err != nil:
		return errors.Annotate(err, "failed to fetch Run").Tag(transient.Tag).Err()
	case !run.IsEnded(r.Status):
		panic("Run status must be final before sending to BQ.")
	}

	a, err := makeAttempt(ctx, &r)
	if err != nil {
		return errors.Annotate(err, "failed to make Attempt").Err()
	}

	// During the migration period when CQDaemon does most checks and triggers
	// builds, CV can't populate all of the fields of Attempt without the
	// information from CQDaemon; so for finished Attempts reported by
	// CQDaemon, we can fill in the remaining fields.
	switch cqda, err := fetchCQDAttempt(ctx, &r); {
	case err != nil:
		return err
	case cqda != nil:
		a = reconcileAttempts(a, cqda)
	}

	eg, ctx := errgroup.WithContext(ctx)
	defer eg.Wait()

	logging.Debugf(ctx, "CV exporting Run to CQ BQ table")
	eg.Go(func() error {
		project := legacyProject
		if env.IsGAEDev {
			project = legacyProjectDev
		}
		return client.SendRow(ctx, cvbq.Row{
			CloudProject: project,
			Dataset:      legacyDataset,
			Table:        legacyTable,
			OperationID:  "run-" + string(id),
			Payload:      a,
		})
	})

	// *Always* also export to the local CV dataset.
	eg.Go(func() error {
		return client.SendRow(ctx, cvbq.Row{
			Dataset:     CVDataset,
			Table:       CVTable,
			OperationID: "run-" + string(id),
			Payload:     a,
		})
	})
	return eg.Wait()
}

func makeAttempt(ctx context.Context, r *run.Run) (*cvbqpb.Attempt, error) {
	// Load CLs and convert them to GerritChanges including submit status.
	runCLs, err := run.LoadRunCLs(ctx, r.ID, r.CLs)
	if err != nil {
		return nil, err
	}
	submittedSet := common.MakeCLIDsSet(r.Submission.GetSubmittedCls()...)
	failedSet := common.MakeCLIDsSet(r.Submission.GetFailedCls()...)
	gerritChanges := make([]*cvbqpb.GerritChange, len(runCLs))
	for i, cl := range runCLs {
		gerritChanges[i] = toGerritChange(cl, submittedSet, failedSet, r.Mode)
	}

	// TODO(crbug/1173168, crbug/1105669): We want to change the BQ
	// schema so that StartTime is processing start time and CreateTime is
	// trigger time.
	a := &cvbqpb.Attempt{
		Key:                  r.ID.AttemptKey(),
		LuciProject:          r.ID.LUCIProject(),
		ConfigGroup:          r.ConfigGroupID.Name(),
		ClGroupKey:           computeCLGroupKey(runCLs, false),
		EquivalentClGroupKey: computeCLGroupKey(runCLs, true),
		// Run.CreateTime is trigger time, which corresponds to what CQD sends for
		// StartTime.
		StartTime:     timestamppb.New(r.CreateTime),
		EndTime:       timestamppb.New(r.EndTime),
		GerritChanges: gerritChanges,
		// Builds, Substatus and HasCustomRequirement are not known to CV yet
		// during the migration state, so they should be filled in with Attempt
		// from CQD if possible.
		Builds: nil,
		Status: attemptStatus(ctx, r),
		// TODO(crbug/1114686): Add a new FAILED_SUBMIT substatus, which
		// should be used in the case that some CLs failed to submit after
		// passing checks. (In this case, for backwards compatibility, we
		// will set status = SUCCESS, substatus = FAILED_SUBMIT.)
		Substatus: cvbqpb.AttemptSubstatus_NO_SUBSTATUS,
	}
	return a, nil
}

// toGerritChange creates a GerritChange for the given RunCL.
//
// This includes the submit status of the CL.
func toGerritChange(cl *run.RunCL, submitted, failed common.CLIDsSet, mode run.Mode) *cvbqpb.GerritChange {
	detail := cl.Detail
	ci := detail.GetGerrit().GetInfo()
	gc := &cvbqpb.GerritChange{
		Host:                       detail.GetGerrit().Host,
		Project:                    ci.Project,
		Change:                     ci.Number,
		Patchset:                   int64(detail.Patchset),
		EarliestEquivalentPatchset: int64(detail.MinEquivalentPatchset),
		TriggerTime:                cl.Trigger.Time,
		Mode:                       mode.BQAttemptMode(),
		SubmitStatus:               cvbqpb.GerritChange_PENDING,
	}

	if mode == run.FullRun {
		// Mark the CL submit status as success if it appears in the submitted CLs
		// list, and failure if it does not.
		if _, ok := submitted[cl.ID]; ok {
			gc.SubmitStatus = cvbqpb.GerritChange_SUCCESS
		} else if failed.Has(cl.ID) {
			gc.SubmitStatus = cvbqpb.GerritChange_FAILURE
		} else {
			gc.SubmitStatus = cvbqpb.GerritChange_PENDING
		}
	}

	return gc
}

// fetchCQDAttempt fetches an Attempt from CQDaemon if available.
//
// Returns nil if no Attempt is available.
func fetchCQDAttempt(ctx context.Context, r *run.Run) (*cvbqpb.Attempt, error) {
	v := migration.VerifiedCQDRun{ID: r.ID}
	switch err := datastore.Get(ctx, &v); {
	case err == datastore.ErrNoSuchEntity:
		// A Run may end without a VerifiedCQDRun stored if the Run is canceled.
		logging.Debugf(ctx, "no VerifiedCQDRun found for Run %q", r.ID)
	case err != nil:
		return nil, errors.Annotate(err, "failed to fetch VerifiedCQDRun").Tag(transient.Tag).Err()
	}
	return v.Payload.GetRun().GetAttempt(), nil
}

// reconcileAttempts merges the CV Attempt and CQDaemon Attempt.
//
// Modifies and returns the CV Attempt.
//
// Once CV does the relevant work (keeping track of builds, reading the CL
// description footers, and performing checks) these will no longer have to be
// filled in with the CQDaemon Attempt values.
func reconcileAttempts(a, cqda *cvbqpb.Attempt) *cvbqpb.Attempt {
	// The list of Builds will be known to CV after it starts triggering
	// and tracking builds; until then CQD is the source of truth.
	a.Builds = cqda.Builds
	// Substatus generally indicates a failure reason, which is
	// known once one of the checks fails. CQDaemon may specify
	// a substatus in the case of abort (substatus: MANUAL_CANCEL)
	// or failure (FAILED_TRYJOBS etc.).
	if a.Status == cvbqpb.AttemptStatus_ABORTED || a.Status == cvbqpb.AttemptStatus_FAILURE {
		a.Status = cqda.Status
		a.Substatus = cqda.Substatus
	}
	a.Status = cqda.Status
	a.Substatus = cqda.Substatus
	// The HasCustomRequirement is determined by CL description footers.
	a.HasCustomRequirement = cqda.HasCustomRequirement
	return a
}

// attemptStatus converts a Run status to Attempt status.
func attemptStatus(ctx context.Context, r *run.Run) cvbqpb.AttemptStatus {
	switch r.Status {
	case run.Status_SUCCEEDED:
		return cvbqpb.AttemptStatus_SUCCESS
	case run.Status_FAILED:
		// In the case that the checks passed but not all CLs were submitted
		// successfully, the Attempt will still have status set to SUCCESS for
		// backwards compatibility. Note that r.Submission is expected to be
		// set only if a submission is attempted, meaning all checks passed.
		if r.Submission != nil && len(r.Submission.Cls) != len(r.Submission.SubmittedCls) {
			return cvbqpb.AttemptStatus_SUCCESS
		}
		return cvbqpb.AttemptStatus_FAILURE
	case run.Status_CANCELLED:
		return cvbqpb.AttemptStatus_ABORTED
	default:
		logging.Errorf(ctx, "Unexpected attempt status %q", r.Status)
		return cvbqpb.AttemptStatus_ATTEMPT_STATUS_UNSPECIFIED
	}
}

// computeCLGroupKey constructs keys for ClGroupKey and the related
// EquivalentClGroupKey.
//
// These are meant to be opaque keys unique to particular set of CLs and
// patchsets for the purpose of grouping together runs for the same sets of
// patchsets. if isEquivalent is true, then the "min equivalent patchset" is
// used instead of the latest patchset, so that trivial patchsets such as minor
// rebases and CL description updates don't change the key.
func computeCLGroupKey(cls []*run.RunCL, isEquivalent bool) string {
	sort.Slice(cls, func(i, j int) bool {
		// ExternalID includes host and change number but not patchset; but
		// different patchsets of the same CL will never be included in the
		// same list, so sorting on only ExternalID is sufficient.
		return cls[i].ExternalID < cls[j].ExternalID
	})
	h := sha256.New()
	// CL group keys are meant to be opaque keys. We'd like to avoid people
	// depending on CL group key and equivalent CL group key sometimes being
	// equal. We can do this by adding a salt to the hash.
	if isEquivalent {
		h.Write([]byte("equivalent_cl_group_key"))
	}
	separator := []byte{0}
	for i, cl := range cls {
		if i > 0 {
			h.Write(separator)
		}
		h.Write([]byte(cl.Detail.GetGerrit().GetHost()))
		h.Write(separator)
		h.Write([]byte(strconv.FormatInt(cl.Detail.GetGerrit().GetInfo().GetNumber(), 10)))
		h.Write(separator)
		if isEquivalent {
			h.Write([]byte(strconv.FormatInt(int64(cl.Detail.GetMinEquivalentPatchset()), 10)))
		} else {
			h.Write([]byte(strconv.FormatInt(int64(cl.Detail.GetPatchset()), 10)))
		}
	}
	return hex.EncodeToString(h.Sum(nil)[:8])
}
