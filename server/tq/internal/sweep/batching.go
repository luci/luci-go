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

package sweep

import (
	"context"
	"sync/atomic"
	"time"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/dispatcher"
	"go.chromium.org/luci/common/sync/dispatcher/buffer"

	"go.chromium.org/luci/server/tq/internal"
	"go.chromium.org/luci/server/tq/internal/db"
	"go.chromium.org/luci/server/tq/internal/reminder"
)

// BatchProcessor handles reminders in batches.
type BatchProcessor struct {
	Context   context.Context    // the context to use for processing
	DB        db.DB              // DB to use to fetch reminders from
	Submitter internal.Submitter // knows how to submit tasks

	BatchSize         int // max size of a single reminder batch
	ConcurrentBatches int // how many concurrent batches to process

	ch        dispatcher.Channel[*reminder.Reminder]
	processed int32 // total reminders successfully processed
}

// Start launches background processor goroutines.
func (p *BatchProcessor) Start() error {
	var err error
	p.ch, err = dispatcher.NewChannel[*reminder.Reminder](
		p.Context,
		&dispatcher.Options[*reminder.Reminder]{
			Buffer: buffer.Options{
				MaxLeases:     p.ConcurrentBatches,
				BatchItemsMax: p.BatchSize,
				// Max waiting time to fill the batch.
				BatchAgeMax: 10 * time.Millisecond,
				FullBehavior: &buffer.BlockNewItems{
					// If all workers are busy, block Enqueue.
					MaxItems: p.ConcurrentBatches * p.BatchSize,
				},
			},
		},
		p.processBatch,
	)
	if err != nil {
		return errors.Fmt("invalid sweeper configuration: %w", err)
	}
	return nil
}

// Stop waits until all enqueues reminders are processed and then stops the
// processor.
//
// Returns the total number of successfully processed reminders.
func (p *BatchProcessor) Stop() int {
	p.ch.Close()
	<-p.ch.DrainC
	return int(atomic.LoadInt32(&p.processed))
}

// Enqueue adds reminder to the to-be-processed queue.
//
// Must be called only between Start and Stop. Drops reminders on the floor if
// the context is canceled.
func (p *BatchProcessor) Enqueue(ctx context.Context, r []*reminder.Reminder) {
	for _, rem := range r {
		select {
		case p.ch.C <- rem:
		case <-ctx.Done():
			return
		}
	}
}

// processBatch called concurrently to handle a single batch of items.
//
// Logs errors inside, doesn't return them.
func (p *BatchProcessor) processBatch(data *buffer.Batch[*reminder.Reminder]) error {
	batch := make([]*reminder.Reminder, len(data.Data))
	for i, d := range data.Data {
		batch[i] = d.Item
	}
	count, err := internal.SubmitBatch(p.Context, p.Submitter, p.DB, batch)
	if err != nil {
		logging.Errorf(p.Context, "Processed only %d/%d reminders: %s", count, len(batch), err)
	}
	atomic.AddInt32(&p.processed, int32(count))
	return nil
}
