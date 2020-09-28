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
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
)

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

	rejectedPatchSets, err := (&rejectedPatchSetSource{evalRun: r}).Read(ctx)
	if err != nil {
		return nil, err
	}
	progress := progress{Total: len(rejectedPatchSets)}

	eg, ctx := errgroup.WithContext(ctx)
	rejectedPatchSetC := make(chan *RejectedPatchSet)
	eg.Go(func() error {
		defer close(rejectedPatchSetC)
		for _, rp := range rejectedPatchSets {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case rejectedPatchSetC <- rp:
			}
		}
		return ctx.Err()
	})

	var eligibleCount int32
	var rejectedCount int32
	for i := 0; i < r.Concurrency; i++ {
		eg.Go(func() error {
			for rp := range rejectedPatchSetC {
				eligible, wouldReject, err := r.processPatchSet(ctx, rp)
				if err != nil {
					return errors.Annotate(err, "failed to process patchset %s", &rp.Patchset).Err()
				}
				if eligible {
					atomic.AddInt32(&eligibleCount, 1)
					if wouldReject {
						atomic.AddInt32(&rejectedCount, 1)
					}
				}
				progress.Done(ctx)
			}
			return ctx.Err()
		})
	}

	if err := eg.Wait(); err != nil {
		return nil, err
	}
	logging.Infof(ctx, "Processed all patchsets")

	return &Result{
		AnalyzedPatchSets: len(rejectedPatchSets),
		Safety: Safety{
			EligiblePatchSets: int(eligibleCount),
			Rejected:          int(rejectedCount),
		},
	}, nil
}

func (r *evalRun) processPatchSet(ctx context.Context, rp *RejectedPatchSet) (eligible, wouldReject bool, err error) {
	// TODO(crbug.com/1112125): add support for CL stacks.
	// This call returns only files modified in the particular patchset and
	// ignores possible parent CLs that were also tested.

	// TODO(crbug.com/1112125): skip the patchset if it has a ton of failed tests.
	// Most RTS algorithms would reject such a patchset, so it represents noise.

	files, err := r.gerrit.ChangedFiles(ctx, &rp.Patchset)
	switch {
	case psNotFound.In(err):
		// The CL is deleted  => not eligible.
		err = nil
		return

	case err != nil:
		err = errors.Annotate(err, "failed to read changed files of %s", &rp.Patchset).Err()
		return
	}

	// Compare the prediction to facts.
	in := Input{ChangedFiles: files}
	for _, t := range rp.FailedTests {
		in.Test = t
		var out Output
		if out, err = r.Algorithm(ctx, in); err != nil {
			err = errors.Annotate(err, "RTS algorithm failed").Err()
			return
		}
		eligible = true

		if out.ShouldRun {
			// At least one failed test would run => the bad patchset would be rejected.
			wouldReject = true
			return
		}
	}
	return
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
