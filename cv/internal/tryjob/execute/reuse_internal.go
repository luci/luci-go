// Copyright 2022 The LUCI Authors.
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

package execute

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/tryjob"
)

// findReuseInCV returns reusable Tryjob candidates from CV datastore.
func (w *worker) findReuseInCV(ctx context.Context, definitions []*tryjob.Definition) (map[*tryjob.Definition]*tryjob.Tryjob, error) {
	candidates, err := w.queryForCandidates(ctx, definitions)
	switch {
	case err != nil:
		return nil, err
	case len(candidates) == 0:
		return nil, nil
	}

	defs := make([]*tryjob.Definition, 0, len(candidates))
	tryjobIDs := make(common.TryjobIDs, 0, len(candidates))
	for def, tj := range candidates {
		defs = append(defs, def)
		tryjobIDs = append(tryjobIDs, tj.ID)
	}
	tjs, err := w.addCurrentRunToReuse(ctx, tryjobIDs)
	if err != nil {
		return nil, err
	}
	ret := make(map[*tryjob.Definition]*tryjob.Tryjob, len(defs))
	for i, def := range defs {
		ret[def] = tjs[i]
	}
	return ret, nil
}

// queryForCandidates makes a DS query to find Tryjob candidates for reuse that
// have a matching reuse key.
func (w *worker) queryForCandidates(ctx context.Context, definitions []*tryjob.Definition) (map[*tryjob.Definition]*tryjob.Tryjob, error) {
	q := datastore.NewQuery(tryjob.TryjobKind).Eq("ReuseKey", w.reuseKey)
	luciProject := w.run.ID.LUCIProject()
	mode := w.run.Mode
	candidates := make(map[*tryjob.Definition]*tryjob.Tryjob)
	err := datastore.Run(ctx, q, func(tj *tryjob.Tryjob) error {
		switch def := matchDefinitions(tj, definitions); {
		case def == nil:
		case w.knownTryjobIDs.Has(tj.ID):
		case tj.LUCIProject() != luciProject:
			// Ensures Run only reuse the Tryjob that its belonging LUCI
			// Project has access to. This check may give a false negative
			// result but it's good enough, because currently it's very
			// unlikely for a Run from Project A to reuse a Tryjob triggered by
			// a Run from Project B. Project A and Project B should watch a
			// disjoint set of Gerrit refs.
		case canReuseTryjob(ctx, tj, mode) == reuseDenied:
		case tj.EntityCreateTime.IsZero():
			panic(fmt.Errorf("tryjob %d has zero entity create time", tj.ID))
		default:
			if existing, ok := candidates[def]; !ok || tj.EntityCreateTime.After(existing.EntityCreateTime) {
				// Pick the latest one.
				candidates[def] = tj
			}
		}
		return nil
	})
	if err != nil {
		return nil, errors.Annotate(err, "failed to query for reusable tryjobs").Tag(transient.Tag).Err()
	}
	return candidates, nil
}

// matchDefinitions returns a Definition that matches the given Tryjob from the
// given list of Definitions.
func matchDefinitions(tj *tryjob.Tryjob, definitions []*tryjob.Definition) *tryjob.Definition {
	for _, def := range definitions {
		switch {
		case proto.Equal(tj.Definition, def):
			return def
		case def.GetBuildbucket() != nil:
			switch builder := tj.Result.GetBuildbucket().GetBuilder(); {
			case builder == nil:
			case proto.Equal(builder, def.GetBuildbucket().GetBuilder()):
				return def
			case proto.Equal(builder, def.GetEquivalentTo().GetBuildbucket().GetBuilder()):
				return def
			}
		default:
			panic(fmt.Errorf("unknown backend: %T", def.GetBackend()))
		}
	}
	return nil
}

func (w *worker) addCurrentRunToReuse(ctx context.Context, tjIDs common.TryjobIDs) ([]*tryjob.Tryjob, error) {
	var tryjobs []*tryjob.Tryjob
	var innerErr error
	err := datastore.RunInTransaction(ctx, func(ctx context.Context) (err error) {
		defer func() { innerErr = err }()
		tryjobs = make([]*tryjob.Tryjob, len(tjIDs))
		for i, id := range tjIDs {
			tryjobs[i] = &tryjob.Tryjob{ID: id}
		}
		if err := datastore.Get(ctx, tryjobs); err != nil {
			return errors.Annotate(err, "failed to load Tryjob entities").Tag(transient.Tag).Err()
		}
		var toSave []*tryjob.Tryjob
		for _, tj := range tryjobs {
			// Be defensive. Tryjob may already include this Run if previous request
			// failed in the middle.
			if tj.ReusedBy.Index(w.run.ID) < 0 {
				tj.ReusedBy = append(tj.ReusedBy, w.run.ID)
				tj.EVersion++
				tj.EntityUpdateTime = clock.Now(ctx).UTC()
				toSave = append(toSave, tj)
			}
		}
		return tryjob.SaveTryjobs(ctx, toSave, w.rm.NotifyTryjobsUpdated)
	}, nil)
	switch {
	case innerErr != nil:
		return nil, innerErr
	case err != nil:
		return nil, errors.Annotate(err, "failed to commit transaction").Tag(transient.Tag).Err()
	}
	return tryjobs, nil
}
