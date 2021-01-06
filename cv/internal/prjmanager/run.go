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
	"fmt"
	"sort"
	"strconv"
	"time"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"
	"golang.org/x/sync/errgroup"

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
	switch err := rb.load(ctx); {
	case err == errAlreadyCreated:
		return rb.run, nil
	case err != nil:
		return nil, err
	}
	if err := rb.save(ctx); err != nil {
		return nil, err
	}
	return rb.run, nil
}

// load reads latest state from Datastore and verifies creation can proceed.
func (rb *RunBuilder) load(ctx context.Context) error {
	rb.dsBatcher.reset()
	rb.checkRunExists(ctx)
	rb.checkProjectState(ctx)
	rb.checkCLsUnchanged(ctx)
	return rb.dsBatcher.get(ctx)
}

// errAlreadyCreated is an internal error to signal that save is not necessary,
// since the intended Run is already created.
var errAlreadyCreated = errors.New("already created")

// checkRunExists checks if Run already exists in datastore,
// and if it was created by us.
func (rb *RunBuilder) checkRunExists(ctx context.Context) {
	rb.run = &run.Run{ID: rb.runID}
	rb.dsBatcher.register(rb.run, func(err error) error {
		switch {
		case err == datastore.ErrNoSuchEntity:
			// Run doesn't exist, which is expected.
			return nil
		case err != nil:
			return errors.Annotate(err, "failed to load Run entity").Tag(transient.Tag).Err()

		case rb.run.CreationOperationID == rb.OperationID:
			// This is quite likely if prior transaction attempt actually succeeds
			// succeeded on Datastore side, but CV failed to receive an ACK and is now
			// retrying the transaction body.
			logging.Debugf(ctx, "Run(ID:%s) already created by us", rb.runID)
			return errAlreadyCreated
		default:
			// Concurrent request computing RunID at exact same time should not
			// normally happen, so log it as suspicious.
			logging.Warningf(ctx, "Run(ID:%s) already created with OperationID %q", rb.runID, rb.run.CreationOperationID)
			return errors.Reason("Run(ID:%s) already created with OperationID %q", rb.runID, rb.run.CreationOperationID).Err()
		}
	})
}

// checkProjectState checks if projet is running and uses the expected
// ConfigHash.
func (rb *RunBuilder) checkProjectState(ctx context.Context) {
	ps := &ProjectStateOffload{
		Project: datastore.MakeKey(ctx, ProjectKind, rb.LUCIProject),
	}
	rb.dsBatcher.register(ps, func(err error) error {
		switch {
		case err == datastore.ErrNoSuchEntity:
			return errors.Annotate(err, "failed to load ProjectStateOffload").Err()
		case err != nil:
			return errors.Annotate(err, "failed to load ProjectStateOffload").Tag(transient.Tag).Err()
		case ps.Status != Status_STARTED:
			return errors.Reason("project %q status is %s, expected STARTED", rb.LUCIProject, ps.Status.String()).Tag(StateChangedTag).Err()
		case ps.ConfigHash != rb.ConfigGroupID.Hash():
			return errors.Reason("project config is %s, expected %s", ps.ConfigHash, rb.ConfigGroupID.Hash()).Tag(StateChangedTag).Err()
		default:
			return nil
		}
	})
}

// checkCLsUnchanged sets `.cls` with latest Datastore value and verifies thier
// EVersion match what's expected.
func (rb *RunBuilder) checkCLsUnchanged(ctx context.Context) {
	rb.cls = make([]*changelist.CL, len(rb.InputCLs))
	for i, inputCL := range rb.InputCLs {
		rb.cls[i] = &changelist.CL{ID: inputCL.ID}
		i := i
		id := inputCL.ID
		expected := inputCL.ExpectedEVersion
		rb.dsBatcher.register(rb.cls[i], func(err error) error {
			switch {
			case err == datastore.ErrNoSuchEntity:
				return errors.Annotate(err, "CL %d doesn't exist", id).Err()
			case err != nil:
				return errors.Annotate(err, "failed to load CL %d", id).Tag(transient.Tag).Err()
			case rb.cls[i].EVersion != expected:
				return errors.Reason("CL %d changed since EVersion %d", id, expected).Tag(StateChangedTag).Err()
			default:
				return nil
			}
		})
	}
}

// save saves all modified/created Datastore entities.
//
// It may be retried multiple times on failure.
func (rb *RunBuilder) save(ctx context.Context) error {
	rb.dsBatcher.reset()
	// Keep .CreateTime and | .UpdateTime entities the same across all saved
	// entities. Do pre-emptive rounding before Datastore layer does it such
	// rb.run entityRun entity has exactly values as what would be read from
	// Datastore later.
	now := datastore.RoundTime(clock.Now(ctx).UTC())
	rb.registerSaveRun(ctx, now)
	for i := range rb.InputCLs {
		rb.registerSaveRunCL(ctx, i)
		rb.registerSaveCL(ctx, i, now)
	}
	// TODO(tandrii): consider also using rb.dsBatcher for PM notification.
	eg, ctx := errgroup.WithContext(ctx)
	eg.Go(func() error { return rb.savePMNotification(ctx) })
	eg.Go(func() error { return rb.dsBatcher.put(ctx) })
	return eg.Wait()
}

func (rb *RunBuilder) registerSaveRun(ctx context.Context, now time.Time) {
	ids := make([]common.CLID, len(rb.InputCLs))
	for i, cl := range rb.InputCLs {
		ids[i] = cl.ID
	}
	rb.run = &run.Run{
		ID:                  rb.run.ID,
		EVersion:            1,
		CreationOperationID: rb.OperationID,
		CreateTime:          now,
		UpdateTime:          now,
		// EndTime & StartTime intentionally left unset.

		CLs:           ids,
		ConfigGroupID: rb.ConfigGroupID,
		Mode:          rb.Mode,
		Status:        run.Status_PENDING,
		Owner:         rb.Owner,
	}
	rb.dsBatcher.register(rb.run, func(err error) error {
		return errors.Annotate(err, "failed to save Run").Tag(transient.Tag).Err()
	})
}

func (rb *RunBuilder) registerSaveRunCL(ctx context.Context, index int) {
	inputCL := rb.InputCLs[index]
	entity := &run.RunCL{
		Run:     datastore.MakeKey(ctx, run.RunKind, string(rb.run.ID)),
		ID:      inputCL.ID,
		Trigger: inputCL.TriggerInfo,
		Detail:  rb.cls[index].Snapshot,
	}
	rb.dsBatcher.register(entity, func(err error) error {
		return errors.Annotate(err, "failed to save RunCL %d", inputCL.ID).Tag(transient.Tag).Err()
	})
}

func (rb *RunBuilder) registerSaveCL(ctx context.Context, index int, now time.Time) {
	cl := rb.cls[index]
	cl.EVersion++
	cl.UpdateTime = now
	cl.IncompleteRuns.InsertSorted(rb.runID)
	rb.dsBatcher.register(cl, func(err error) error {
		return errors.Annotate(err, "failed to save CL %d", cl.ID).Tag(transient.Tag).Err()
	})
}

func (rb *RunBuilder) savePMNotification(ctx context.Context) error {
	return runCreated(ctx, rb.runID)
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
	entities  []interface{}
	callbacks []func(error) error
}

func (d *dsBatcher) reset() {
	d.entities = d.entities[:0]
	d.callbacks = d.callbacks[:0]
}

// register registers an entity for future get/put and a callback to handle errors.
//
// entity must be a pointer to an entity object.
// callback is called with entity specific error, possibly nil.
// callback must not be nil.
func (d *dsBatcher) register(entity interface{}, callback func(error) error) {
	d.entities = append(d.entities, entity)
	d.callbacks = append(d.callbacks, callback)
}

// get loads entities from datastore and performs error handling.
//
// Aborts and returns the first non-nil error returned by a callback.
// Oherwise, returns nil.
func (d *dsBatcher) get(ctx context.Context) error {
	err := datastore.Get(ctx, d.entities)
	if err == nil {
		return d.execCallbacks(nil)
	}
	switch errs, ok := err.(errors.MultiError); {
	case !ok:
		return errors.Annotate(err, "failed to load entities from Datastore").Tag(transient.Tag).Err()
	case len(errs) != len(d.entities):
		panic(fmt.Errorf("%d errors for %d entities", len(errs), len(d.entities)))
	default:
		return d.execCallbacks(errs)
	}
}

// put saves entities to datastore and performs error handling.
//
// Aborts and returns the first non-nil error returned by a callback.
// Oherwise, returns nil.
func (d *dsBatcher) put(ctx context.Context) error {
	err := datastore.Put(ctx, d.entities)
	if err == nil {
		return d.execCallbacks(nil)
	}
	switch errs, ok := err.(errors.MultiError); {
	case !ok:
		return errors.Annotate(err, "failed to put entities to Datastore").Tag(transient.Tag).Err()
	case len(errs) != len(d.entities):
		panic(fmt.Errorf("%d errors for %d entities", len(errs), len(d.entities)))
	default:
		return d.execCallbacks(errs)
	}
}

func (d *dsBatcher) execCallbacks(errs errors.MultiError) error {
	if errs == nil {
		for _, f := range d.callbacks {
			if err := f(nil); err != nil {
				return err
			}
		}
		return nil
	}

	for i, f := range d.callbacks {
		if err := f(errs[i]); err != nil {
			return err
		}
	}
	return nil
}
