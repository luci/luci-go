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

	"go.chromium.org/luci/common/errors"
	"golang.org/x/time/rate"
)

// normalize validates that Options is well formed and populates defaults which
// are missing.
func (o *Options) normalize(ctx context.Context) error {
	if o.ErrorFn == nil {
		o.ErrorFn = defaultErrorFnFactory(ctx)
	}
	if o.DropFn == nil {
		o.DropFn = defaultDropFnFactory(ctx, o.Buffer.FullBehavior)
	}
	if o.QPSLimit == nil {
		o.QPSLimit = rate.NewLimiter(1, 1)
	}

	if o.MaxSenders == 0 {
		o.MaxSenders = Defaults.MaxSenders
	} else if o.MaxSenders < 1 {
		return errors.Reason("MaxSenders must be > 0: got %d", o.MaxSenders).Err()
	}

	return nil
}
