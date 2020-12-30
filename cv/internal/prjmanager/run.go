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

package prjmanager

import (
	"context"
	"crypto/sha256"
	"sort"
	"strconv"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"
	"golang.org/x/sync/errgroup"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/run"
)

// RunBuilder creates a new Run.
//
// If Expected<...> parameters differ from what's read from Datastore
// during transaction, the creation is aborted with error tagged with
// StateChangedTag. See RunBuilder.Create doc.
type RunBuilder struct {
	// All public fields are required.

	LUCIProject string
	// TODO(tandrii): support triggering via API calls by validating Run creation
	// request against the latest config *before* transaction.
	ExpectedConfigHash string
	CLs                []RunBuilderCL

	// Internal state: pre-computed once before transaction starts.

	runIDBuilder struct {
		version int
		digest  []byte
	}

	// Internal state: computed during transaction.
	// Since transactions are retried, this can be re-set multiple times.
	runID      common.RunID
	clsBatchOp *changelist.BatchOp
}

// RunBuilderCL is a helper struct for per-CL input for run creation.
type RunBuilderCL struct {
	ID               common.CLID
	ExpectedEVersion int
	TriggerInfo      *run.Trigger
	Snapshot         *changelist.Snapshot // only needed for compat with CQDaemon.
}

// StateChangedTag is an error tag used to indicate that state read from Datastore
// differs from the expected state.
var StateChangedTag = errors.BoolTag{Key: errors.NewTagKey("the task should be dropped")}

// Create atomically creates a new Run.
//
// Returns the newly created Run.
//
// Returns 3 kinds of errors:
//
//   * tagged with transient.Tag, meaning it's reasonable to retry.
//     Typically due to contention on simultaneously updated CL entity or
//     transient Datastore Get/Put problem.
//
//   * tagged with StateChangedTag.
//     This means the desired Run might still be possible to create,
//     but it must be re-validated against updated CLs or Project config
//     version.
//
//   * the rest of errors are non retryable and typically indicate a bug or
//     severe misconfiguration. For example, lack of ProjectStateOffload entity.
func (rb *RunBuilder) Create(ctx context.Context) (*run.Run, error) {
	if err := rb.prepare(ctx); err != nil {
		return nil, err
	}
	var ret *run.Run
	err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		var err error
		ret, err = rb.createTransactionally(ctx)
		return err
	}, nil)
	if err != nil {
		return nil, errors.Annotate(err, "failed to create run").Tag(transient.Tag).Err()
	}
	return ret, nil
}

func (rb *RunBuilder) prepare(ctx context.Context) error {
	// TODO(tandrii): implement:
	// * compute clsDigest.
	return nil
}

func (rb *RunBuilder) createTransactionally(ctx context.Context) (*run.Run, error) {
	rb.computeRunID(ctx)
	// TODO(tandrii): implement
	return nil, nil
}

// load reads latest state from Datastore and verifies creation can proceed.
func (rb *RunBuilder) load(ctx context.Context) error {
	eg, ctx := errgroup.WithContext(ctx)
	eg.Go(func() error { return rb.loadCLs(ctx) })
	eg.Go(func() error { return rb.loadOthers(ctx) })
	return eg.Wait()
}

// loadCLs reads CLs from Datastore and validates ExpectedIncompleteRuns.
//
// Sets .CLsBatchOp on success.
func (rb *RunBuilder) loadCLs(ctx context.Context) error {
	// TODO(tandrii): implement
	return nil
}

// loadOthers reads from datastore non-CL entities.
//
// loadOthers does a bunch of things in order to batch multiple reads into 1
// Datastore RPC.
func (rb *RunBuilder) loadOthers(ctx context.Context) error {
	// TODO(tandrii): implement
	return nil
}

// computeCLsDigest populates rb.runIDBuilder for use by computeRunID.
func (rb *RunBuilder) computeCLsDigest() {
	// 1st version uses truncated CQDaemon's `attempt_key_hash`
	// aimed for ease of comparison / log grepping during migration.
	// However, this assumes Gerrit CLs and requires having changelist.Snapshot
	// pre-loaded.
	// TODO(tandrii): after migration is over, change to hash CLIDs instead, since
	// it's good enough for the purpose of avoiding spurious collision of RunIDs.
	rb.runIDBuilder.version = 1

	triples := make([]struct {
		host      string
		number    int64
		unixUSecs int64
	}, len(rb.CLs))
	for i, cl := range rb.CLs {
		triples[i].host = cl.Snapshot.GetGerrit().GetHost()
		triples[i].number = cl.Snapshot.GetGerrit().GetInfo().GetNumber()
		secs := cl.TriggerInfo.GetTime().GetSeconds()
		// CQDaemon truncates ns precision to microseconds in gerrit_util.parse_time.
		microsecs := int64(cl.TriggerInfo.GetTime().GetNanos() / 1000)
		triples[i].unixUSecs = (secs*1000*1000 + microsecs)
	}
	sort.Slice(triples, func(i, j int) bool {
		a, b := triples[i], triples[j]
		switch {
		case a.host < b.host:
			return true
		case a.host > b.host:
			return false
		case a.number < b.number:
			return true
		case a.number > b.number:
			return false
		default:
			return a.unixUSecs < b.unixUSecs
		}
	})
	separtor := []byte{0}
	h := sha256.New224()
	for i, t := range triples {
		if i > 0 {
			h.Write(separtor)
		}
		h.Write([]byte(t.host))
		h.Write(separtor)
		h.Write([]byte(strconv.FormatInt(t.number, 10)))
		h.Write(separtor)
		h.Write([]byte(strconv.FormatInt(t.unixUSecs, 10)))
	}
	rb.runIDBuilder.digest = h.Sum(nil)
}

// computeRunID generates and saves new Run ID in `.runID`.
func (rb *RunBuilder) computeRunID(ctx context.Context) {
	b := &rb.runIDBuilder
	rb.runID = common.MakeRunID(rb.LUCIProject, clock.Now(ctx), b.version, b.digest)
}
