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

	"golang.org/x/time/rate"

	"go.chromium.org/luci/common/errors"
)

// normalize validates that Options is well formed and populates defaults which
// are missing.
func (o *Options) normalize(ctx context.Context) error {
	if o.ItemSizeFunc == nil {
		if o.Buffer.BatchSizeMax > 0 {
			return errors.New("Buffer.BatchSizeMax > 0 but ItemSizerFunc == nil")
		}
	}

	if o.ErrorFn == nil {
		o.ErrorFn = defaultErrorFnFactory(ctx)
	}
	if o.DropFn == nil {
		o.DropFn = defaultDropFnFactory(ctx, o.Buffer.FullBehavior)
	}

	if o.QPSLimit == nil {
		o.QPSLimit = rate.NewLimiter(rate.Inf, 0)
	}
	if o.QPSLimit.Limit() != rate.Inf && o.QPSLimit.Burst() < 1 {
		return errors.Reason(
			"QPSLimit has burst size < 1, but a non-infinite rate: %d",
			o.QPSLimit.Burst()).Err()
	}

	return nil
}
