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
	"fmt"
	"io"
	"sync"
	"time"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
)

// Result is the result of evaluation.
type Result struct {
	Safety    Safety
	Precision Precision
}

type evalRun struct {
	Eval

	auth   *auth.Authenticator
	gerrit *gerritClient

	startTime time.Time
	endTime   time.Time
}

func (r *evalRun) run(ctx context.Context) (*Result, error) {
	if err := r.Init(ctx); err != nil {
		return nil, err
	}

	// Evaluate safety and precision sequentially because
	// individually they already use all CPUs,
	// and running sequentially improves log readability.

	safety, err := r.evaluateSafety(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "failed to evaluate safety").Err()
	}

	precision, err := r.evaluatePrecision(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "failed to evaluate precision").Err()
	}

	return &Result{
		Safety:    *safety,
		Precision: *precision,
	}, nil
}

// progress prints the number of processed patchsets at most once per second.
type progress struct {
	Total int

	done       int
	mu         sync.Mutex
	lastReport time.Time
}

func (p *progress) Done(ctx context.Context) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.done++

	if clock.Since(ctx, p.lastReport) < time.Second {
		return
	}

	if !p.lastReport.IsZero() {
		logging.Infof(ctx, "Processing patchset: %5d/%d\n", p.done, p.Total)
	}
	p.lastReport = clock.Now(ctx)
}

// Print prints the results to w.
func (r *Result) Print(w io.Writer) (err error) {
	switch {
	case r.Safety.AnalyzedPatchSets == 0:
		_, err = fmt.Printf("Evaluation failed: patchsets not found\n")

	case r.Safety.EligiblePatchSets == 0:
		_, err = fmt.Printf("Evaluation failed: all %d patchsets are ineligible.\n", r.Safety.AnalyzedPatchSets)

	default:
		fmt.Printf("Total analyzed patchsets: %d\n", r.Safety.AnalyzedPatchSets)
		_, err = fmt.Printf("Safety score: %.2f (%d/%d)\n",
			float64(r.Safety.Rejected)/float64(r.Safety.EligiblePatchSets),
			r.Safety.Rejected,
			r.Safety.EligiblePatchSets,
		)
	}

	// TODO(crbug.com/1112125): print precision.
	return
}
