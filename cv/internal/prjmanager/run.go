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

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/config"
	"go.chromium.org/luci/cv/internal/run"
)

// RunBuilder creates a new Run.
//
// If Expected<...> parameters differ from what's read from Datastore
// during transaction, the creation is aborted with error tagged with
// StateChangedTag. See RunBuilder.Create doc.
type RunBuilder struct {
	// All public fields are required.

	// LUCIProject. Required.
	LUCIProject string
	// ConfigGroupID for the Run. Required.
	//
	// TODO(tandrii): support triggering via API calls by validating Run creation
	// request against the latest config *before* transaction.
	ConfigGroupID config.ConfigGroupID
	// InputCLs will reference the newly created Run via their IncompleteRuns field,
	// and Run's RunCL entities will reference these InputCLs back. Required.
	InputCLs []RunBuilderCL
	// Mode is the run's mode. Required.
	Mode run.Mode
	// Owner is the Run Owner. Required.
	Owner identity.Identity
	// OperationID is an arbitrary string uniquely identifying this creation
	// attempt.
	//
	// TODO(tandrii): for CV API, record this ID in a separate entity index by
	// this ID for full idempotency of CV API.
	OperationID string

	// Internal state: pre-computed once before transaction starts.

	runIDBuilder struct {
		version int
		digest  []byte
	}

	// Internal state: computed during transaction.
	// Since transactions are retried, these can be re-set multiple times.

	// dsBatcher flattens multiple Datastore Get/Put calls into a single call.
	dsBatcher dsBatcher
	// runID is computed runID at the time beginning of a transaction, since RunID
	// depends on time.
	runID common.RunID
	// cls are the read & updated CLs.
	cls []*changelist.CL
	// run stores the resulting Run, eventually returned by Create().
	run *run.Run
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
//   * all other errors are non retryable and typically indicate a bug or severe
//     misconfiguration. For example, lack of ProjectStateOffload entity.
func (rb *RunBuilder) Create(ctx context.Context) (*run.Run, error) {
	if err := rb.prepare(ctx); err != nil {
		return nil, err
	}
	var ret *run.Run
	var innerErr error
	err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		ret, innerErr = rb.createTransactionally(ctx)
		return innerErr
	}, nil)
	switch {
	case innerErr != nil:
		return nil, innerErr
	case err != nil:
		return nil, errors.Annotate(err, "failed to create run").Tag(transient.Tag).Err()
	default:
		return ret, nil
	}
}

func (rb *RunBuilder) prepare(ctx context.Context) error {
	switch {
	case rb.LUCIProject == "":
		panic("LUCIProject is required")
	case rb.ConfigGroupID == "":
		panic("ConfigGroupID is required")
	case len(rb.InputCLs) == 0:
		panic("At least 1 CL is required")
	case rb.Mode == "":
		panic("Mode is required")
	case rb.Owner == "":
		panic("Owner is required")
	case rb.OperationID == "":
		panic("OperationID is required")
	}
	for _, cl := range rb.InputCLs {
		if cl.ExpectedEVersion == 0 || cl.ID == 0 || cl.Snapshot == nil || cl.TriggerInfo == nil {
			panic("Each CL field is required")
		}
	}
	rb.computeCLsDigest()
	return nil
}

func (rb *RunBuilder) createTransactionally(ctx context.Context) (*run.Run, error) {
	rb.computeRunID(ctx)
	// TODO(tandrii): implement.
	return rb.run, nil
}

// load reads latest state from Datastore and verifies creation can proceed.
func (rb *RunBuilder) load(ctx context.Context) error {
	// TODO(tandrii): implement.
	return nil
}

func (rb *RunBuilder) save(ctx context.Context) error {
	// TODO(tandrii): implement.
	return nil
}

// computeCLsDigest populates `.runIDBuilder` for use by computeRunID.
func (rb *RunBuilder) computeCLsDigest() {
	// 1st version uses truncated CQDaemon's `attempt_key_hash`
	// aimed for ease of comparison / log grepping during migration.
	// However, this assumes Gerrit CLs and requires having changelist.Snapshot
	// pre-loaded.
	// TODO(tandrii): after migration is over, change to hash CLIDs instead, since
	// it's good enough for the purpose of avoiding spurious collision of RunIDs.
	rb.runIDBuilder.version = 1

	triples := make([]struct {
		host          string
		number        int64
		unixMicrosecs int64
	}, len(rb.InputCLs))
	for i, cl := range rb.InputCLs {
		triples[i].host = cl.Snapshot.GetGerrit().GetHost()
		triples[i].number = cl.Snapshot.GetGerrit().GetInfo().GetNumber()
		secs := cl.TriggerInfo.GetTime().GetSeconds()
		// CQDaemon truncates ns precision to microseconds in gerrit_util.parse_time.
		microsecs := int64(cl.TriggerInfo.GetTime().GetNanos() / 1000)
		triples[i].unixMicrosecs = (secs*1000*1000 + microsecs)
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
			return a.unixMicrosecs < b.unixMicrosecs
		}
	})
	separator := []byte{0}
	h := sha256.New224()
	for i, t := range triples {
		if i > 0 {
			h.Write(separator)
		}
		h.Write([]byte(t.host))
		h.Write(separator)
		h.Write([]byte(strconv.FormatInt(t.number, 10)))
		h.Write(separator)
		h.Write([]byte(strconv.FormatInt(t.unixMicrosecs, 10)))
	}
	rb.runIDBuilder.digest = h.Sum(nil)
}

// computeRunID generates and saves new Run ID in `.runID`.
func (rb *RunBuilder) computeRunID(ctx context.Context) {
	b := &rb.runIDBuilder
	rb.runID = common.MakeRunID(rb.LUCIProject, clock.Now(ctx), b.version, b.digest)
}

// dsBatcher faciliates processing of many different kind of entities in a
// single Get/Put operation while handling errors in entity-specific code.
type dsBatcher struct {
	// TODO(tandrii): implement.
}
