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
	"time"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
)

var (
	defaultErrorFn = func(ctx context.Context, failedBatch *Batch, err error) (retry bool) {
		logging.Errorf(
			ctx,
			"dispatcher.Channel: failed to send Batch(len(Data): %d, Meta: %q): %s",
			len(failedBatch.Data), failedBatch.Meta, err)

		return transient.Tag.In(err)
	}

	defaultConcurrencyOptions = ConcurrencyOptions{
		MaxSenders: 1,
		MaxQPS:     1.0,
	}

	defaultRetryOptions = RetryOptions{
		InitialSleep:  200 * time.Millisecond,
		MaxSleep:      60 * time.Second,
		BackoffFactor: 1.2,
		Limit:         0,
	}

	defaultBatchOptions = BatchOptions{
		MaxSize:     20,
		MaxDuration: 10 * time.Second,
	}

	defaultBufferOptions = BufferOptions{
		MaxSize:      1000,
		FullBehavior: BlockNewData,
	}
)
