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
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/dispatcher/buffer"
)

type workerResult struct {
	batch *buffer.LeasedBatch
	err   error
}

func (c *Channel) worker(id int) {
	ctx := logging.SetField(c.ctx, "dispatcher.worker", id)

	for {

		logging.Debugf(ctx, "getting work")
		select {
		case <-ctx.Done():
			return

		case batch, ok := <-c.batchCh:
			if !ok {
				logging.Debugf(ctx, "dead")
				return
			}

			logging.Debugf(ctx, "getting work token")
			c.opts.QPSLimit.Wait(ctx)

			logging.Debugf(ctx, "processing batch: len == %d", len(batch.Data))
			rslt := &workerResult{
				batch: batch,
				err:   c.send(batch.Batch),
			}
			logging.Debugf(ctx, "  done, sending")
			c.resultCh <- rslt
		}
	}
}
