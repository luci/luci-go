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

package impl

import (
	"context"
	"fmt"

	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/cv/internal/dsset"
	"go.chromium.org/luci/cv/internal/prjmanager"
	"go.chromium.org/luci/cv/internal/prjmanager/internal"
)

func init() {
	internal.PokePMTaskRef.AttachHandler(
		func(ctx context.Context, payload proto.Message) error {
			task := payload.(*internal.PokePMTask)
			switch err := pokePMTask(ctx, task.GetLuciProject()); {
			case err == nil:
				return nil
			case !transient.Tag.In(err):
				err = tq.Fatal.Apply(err)
				fallthrough
			default:
				errors.Log(ctx, err)
				// TODO(tandrii): avoid retries iff we know a new task was already
				// scheduled for the next second.
				return err
			}
		},
	)
}

func pokePMTask(ctx context.Context, luciProject string) error {
	ctx = logging.SetField(ctx, "project", luciProject)

	var state *state
	var expectedEversion int
	var tr *triageResult
	d := internal.NewDSSet(ctx, luciProject)
	eg, ectx := errgroup.WithContext(ctx)
	eg.Go(func() (err error) {
		state, _, expectedEversion, err = loadState(ectx, luciProject)
		return
	})
	eg.Go(func() (err error) {
		tr, err = triage(ectx, d)
		return
	})
	if err := eg.Wait(); err != nil {
		return err
	}

	// Compute resulting state before transaction.
	emutations, changed, err := prepare(ctx, luciProject, tr, state)
	switch {
	case err != nil:
		return err
	case len(emutations) == 0:
		return nil // nothing to do.
	}

	err = datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		_, p, latestEversion, err := loadState(ctx, luciProject)
		switch {
		case err != nil:
			return err
		case latestEversion != expectedEversion:
			return errors.Reason("Concurrent modification: EVersion read %d, but expected %d",
				latestEversion, expectedEversion).Tag(transient.Tag).Err()
		}
		popOp, err := d.BeginPop(ctx, tr.listing)
		if err != nil {
			return err
		}

		for _, emut := range emutations {
			switch err := emut.apply(ctx, popOp); {
			case err != nil && !transient.Tag.In(err):
				logging.Errorf(ctx, "FIXME: only transient errors expected: %s", err)
				fallthrough
			case err != nil:
				return err
			}
		}

		if changed {
			state.toProjectEntity(p)
			p.EVersion = p.EVersion + 1
			p.UpdateTime = clock.Now(ctx).UTC()
			if err := datastore.Put(ctx, p); err != nil {
				return errors.Annotate(err, "failed to put Project").Tag(transient.Tag).Err()
			}
		}
		return dsset.FinishPop(ctx, popOp)
	}, nil)
	if err != nil {
		// Unconditionally mark error as transient. If op.applyTrans needs to return
		// non-transient errors, this code needs changing.
		return errors.Annotate(err, "failed to commit mutation of %q", luciProject).Tag(
			transient.Tag).Err()
	}
	return nil
}

// emutation is executed during transaction.
//
// It combines `mutation` on state with consumption of incoming events.
type emutation struct {
	mut mutation
	// popIDs are popped from dsset if `mut` succeeds.
	//
	// If event was already popped before, it is silently ignored.
	// TODO(tandrii): add mustConsumeIDs if exactly-once consumption semantics is
	// necessary.
	popIDs []string
}

func (e *emutation) apply(ctx context.Context, p *dsset.PopOp) error {
	if err := e.mut.apply(ctx); err != nil {
		return err
	}
	for _, id := range e.popIDs {
		_ = p.Pop(id) // silently ignore.
	}
	return nil
}

func loadState(ctx context.Context, luciProject string) (
	*state, *prjmanager.Project, int /*expected EVersion*/, error) {
	p := &prjmanager.Project{ID: luciProject}
	switch err := datastore.Get(ctx, p); {
	case err == datastore.ErrNoSuchEntity:
		return &state{}, p, 0, nil
	case err != nil:
		return nil, nil, 0, errors.Annotate(err, "failed to get %q", luciProject).Tag(
			transient.Tag).Err()
	default:
		return fromProjectEntity(p), p, p.EVersion, nil
	}
}

// triageResult is the result of the triage of the incoming events.
type triageResult struct {
	listing      *dsset.Listing
	updateConfig []string // IDs of events from listing.
}

// triage triages incoming events.
func triage(ctx context.Context, d *dsset.Set) (*triageResult, error) {
	listing, err := d.List(ctx)
	if err != nil {
		return nil, err
	}
	// Failing to cleanup already processed events is a hard error, since it slows
	// down future List calls.
	if err := dsset.CleanupGarbage(ctx, listing.Garbage); err != nil {
		return nil, err
	}
	tr := &triageResult{listing: listing}
	for _, item := range listing.Items {
		tr.triage(ctx, item)
	}
	return tr, nil
}

func (tr *triageResult) triage(ctx context.Context, item dsset.Item) {
	e := &internal.Event{}
	if err := proto.Unmarshal(item.Value, e); err != nil {
		// This is a bug in code or data corruption.
		// There is no way to recover on its own.
		logging.Errorf(ctx, "CRITICAL: failed to deserialize event %q: %s", item.ID, err)
		panic(err)
	}
	switch e.GetEvent().(type) {
	case *internal.Event_UpdateConfig:
		tr.updateConfig = append(tr.updateConfig, item.ID)
	default:
		panic(fmt.Errorf("unknown event: %T [id=%q]", e.GetEvent(), item.ID))
	}
}

// preMut is executed before transaction.
type preMut func(context.Context, *machine) (emutation, error)

func (tr *triageResult) iterateEvents() (ret []preMut) {
	if len(tr.updateConfig) > 0 {
		ret = append(ret, func(ctx context.Context, m *machine) (e emutation, err error) {
			e.mut, err = m.updateConfig(ctx)
			e.popIDs = tr.updateConfig
			return
		})
	}
	return
}

func prepare(ctx context.Context, luciProject string, tr *triageResult, state *state) (
	emuts []emutation, changed bool, err error) {
	ctx = logging.SetField(ctx, "prepare", "")
	m := machine{
		luciProject: luciProject,
		dirty:       false,
		state:       state,
	}
	for _, preMut := range tr.iterateEvents() {
		switch emut, err := preMut(ctx, &m); {
		case err != nil:
			// TODO(tandrii): proceed saving partial success if len(ops)>0.
			return nil, false, err
		default:
			emuts = append(emuts, emut)
		}
	}
	return emuts, m.dirty, nil
}
