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

package eval

import (
	"context"

	"go.chromium.org/luci/common/errors"

	evalpb "go.chromium.org/luci/rts/presubmit/eval/proto"
)

type analyzedRejection struct {
	*evalpb.Rejection
	// Affectedness are affectedness for each element of Rejection.FailedTestVariants,
	// in the same order.
	Affectedness AffectednessSlice
	// Closest is the closest element in Affectedness.
	// See also AffectednessSlice.closest().
	Closest Affectedness
}

func analyzeRejections(ctx context.Context, rejC <-chan *evalpb.Rejection, strategy Strategy, callback func(context.Context, analyzedRejection) error) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case rej, ok := <-rejC:
			if !ok {
				return nil
			}

			// TODO(crbug.com/1112125): skip the patchset if it has a ton of failed tests.
			// Most selection strategies would reject such a patchset, so it represents noise.

			// Select tests.
			in := Input{
				TestVariants: rej.FailedTestVariants,
			}
			in.ensureChangedFilesInclude(rej.Patchsets...)
			out := &Output{
				TestVariantAffectedness: make(AffectednessSlice, len(in.TestVariants)),
			}
			if err := strategy(ctx, in, out); err != nil {
				return errors.Annotate(err, "the selection strategy failed").Err()
			}

			closest, err := out.TestVariantAffectedness.closest()
			if err != nil {
				return err
			}

			err = callback(ctx, analyzedRejection{
				Rejection:    rej,
				Affectedness: out.TestVariantAffectedness,
				Closest:      out.TestVariantAffectedness[closest],
			})
			if err != nil {
				return err
			}
		}
	}
}
