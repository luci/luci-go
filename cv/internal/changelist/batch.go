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

package changelist

import (
	"context"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/common"
)

// UpdateBatch initiates a transactional update to several CLs.
//
// Usage:
//
//   err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
//		 batchOp, err := UpdateBatch(ctx, ids)
//		 if err != nil {
//       return err
//     }
//		 for _, cl := range batchOp.CLs {
//			 // modify CL fields, except .UpdateTime and .EVersion.
//		 }
//		 if err := batchOp.Save(ctx); err != nil {
//       return err
//     }
//     // do other stuff referencing final .EVersion and .UpdateTime fields.
//     return nil
//   }, nil)
//
func UpdateBatch(ctx context.Context, ids ...common.CLID) (*BatchOp, error) {
	trans := datastore.CurrentTransaction(ctx)
	if trans == nil {
		panic("must be called in a transaction")
	}
	cls := make([]*CL, len(ids))
	for i, id := range ids {
		cls[i] = &CL{ID: id}
	}
	err := datastore.Get(ctx, cls)
	errs, multiple := err.(errors.MultiError)
	switch {
	case err == nil:
	case !multiple:
		// Probably a misconfiguration, e.g. no Datastore installed in context.
		return nil, errors.Annotate(err, "failed to load CLs").Err()
	default:
		notFound := 0
		for _, oneErr := range errs {
			if oneErr == datastore.ErrNoSuchEntity {
				notFound++
			}
		}
		if notFound == 0 {
			return nil, errors.Annotate(errs.First(), "failed to load CLs").Tag(transient.Tag).Err()
		}
		return nil, errors.Reason("%d out of %d CLs don't exist", notFound, len(ids)).Err()
	}

	b := &BatchOp{CLs: cls, trans: trans}
	b.recordOriginals()
	return b, nil
}

// BatchOp faciliates transactional update of several CLs.
type BatchOp struct {
	CLs []*CL

	// The below fields are just to verify correctness of usage.
	originals []CL
	trans     datastore.Transaction
}

func (b *BatchOp) Save(ctx context.Context) error {
	b.ensureCorrectUsage(ctx)
	now := clock.Now(ctx).UTC()
	for _, cl := range b.CLs {
		cl.Snapshot.PanicIfNotValid()
		cl.EVersion += 1
		cl.UpdateTime = now
	}

	err := datastore.Put(ctx, b.CLs)
	if err != nil {
		return errors.Annotate(err, "failed to save CLs").Tag(transient.Tag).Err()
	}
	return nil
}

// recordOriginals shallow copies read CLs for use in ensureCorrectUsage.
func (b *BatchOp) recordOriginals() {
	b.originals = make([]CL, 0, len(b.CLs))
	for _, cl := range b.CLs {
		b.originals = append(b.originals, *cl)
	}
}

func (b *BatchOp) ensureCorrectUsage(ctx context.Context) {
	if b.originals == nil {
		panic("Incorrect BatchOp usage, must be returned by UpdateBatch")
	}
	if b.trans == nil {
		panic("Second call to BatchOp.Save not allowed")
	}
	if b.trans != datastore.CurrentTransaction(ctx) {
		panic("must be called in the same transaction as UpdateBatch")
	}
	b.trans = nil

	for i, cl := range b.CLs {
		o := &b.originals[i]
		if cl.ID != o.ID {
			panic("reordering CLs slice not allowed")
		}
		if cl.ExternalID != o.ExternalID || cl.EVersion != o.EVersion || cl.UpdateTime != o.UpdateTime {
			panic("modifying CLs {ExternalID, EVersion, UpdateTime} not allowed")
		}
	}
}
