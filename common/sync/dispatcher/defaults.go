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
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/sync/dispatcher/buffer"
)

// Defaults defines the defaults for Options when it contains 0-valued
// fields.
//
// DO NOT ASSIGN/WRITE TO THIS STRUCT.
var Defaults = Options{
	ErrorFn: func(ctx context.Context, failedBatch *buffer.Batch, err error) (retry bool) {
		logging.Errorf(
			ctx,
			"dispatcher.Channel: failed to send Batch(len(Data): %d, Meta: %q): %s",
			len(failedBatch.Data), failedBatch.Meta, err)

		return transient.Tag.In(err)
	},

	MaxSenders: 1,
	MaxQPS:     1.0,
}
