// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package taskqueue

import (
	"golang.org/x/net/context"
)

// Batcher is an augmentation to the top-level taskqueue API that processes
// functions in batches. This can be used to avoid per-operation timeouts that
// the top-level API is subject to.
type Batcher struct {
	// Callback, if not nil, is called in between batch iterations. If the
	// callback returns an error, the error will be returned by the top-level
	// operation, and no further batches will be executed.
	//
	// When querying, the Callback will be executed in between query operations,
	// meaning that the time consumed by the callback will not run the risk of
	// timing out any individual query.
	Callback func(context.Context) error

	// Size is the batch size. If it's <= 0, a default batch size will be chosen
	// based on the batching function and the implementation's constraints.
	Size int
}

// Add puts the specified tasks into the designated queue in batches. See the
// top-level Add for more semantics.
//
// If the specified batch size is <= 0, the current implementation's
// MaxAddSize constraint will be used.
func (b *Batcher) Add(c context.Context, queueName string, tasks ...*Task) error {
	raw := rawWithFilters(c, applyBatchAddFilter(b))
	return addRaw(raw, queueName, tasks)
}

func (b *Batcher) runCallback(c context.Context) error {
	if b.Callback == nil {
		return nil
	}
	return b.Callback(c)
}

type batchAddFilter struct {
	RawInterface

	b  *Batcher
	ic context.Context
}

func applyBatchAddFilter(b *Batcher) RawFilter {
	return func(ic context.Context, rds RawInterface) RawInterface {
		return &batchAddFilter{
			RawInterface: rds,
			b:            b,
			ic:           ic,
		}
	}
}

func (baf *batchAddFilter) AddMulti(tasks []*Task, queueName string, cb RawTaskCB) error {
	// Determine batch size.
	batchSize := baf.b.Size
	if batchSize <= 0 {
		batchSize = baf.Constraints().MaxAddSize
	}
	if batchSize <= 0 {
		return baf.RawInterface.AddMulti(tasks, queueName, cb)
	}

	for len(tasks) > 0 {
		count := batchSize
		if count > len(tasks) {
			count = len(tasks)
		}

		if err := baf.RawInterface.AddMulti(tasks[:count], queueName, cb); err != nil {
			return err
		}

		tasks = tasks[count:]
		if len(tasks) > 0 {
			if err := baf.b.runCallback(baf.ic); err != nil {
				return err
			}
		}
	}
	return nil
}
