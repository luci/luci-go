// Copyright 2019 The LUCI Authors.
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

package dispatcher

import (
	"context"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/dispatcher/buffer"
)

type workerResult struct {
	batch *buffer.LeasedBatch
	err   error
}

func (state *coordinatorState) worker(ctx context.Context, id int, send SendFn) {
	dbg := logging.Get(logging.SetField(ctx, "dispatcher.worker", id)).Debugf

	for {
		dbg("awaiting work")
		select {
		case <-ctx.Done():
			dbg("  dead")
			return

		case batch, ok := <-state.batchCh:
			if !ok {
				dbg("  dead")
				return
			}

			dbg("  processing batch: len == %d", len(batch.Data))
			err := send(batch.Batch)
			dbg("  done, sending")

			state.resultCh <- &workerResult{
				batch: batch,
				err:   err,
			}
		}
	}
}
