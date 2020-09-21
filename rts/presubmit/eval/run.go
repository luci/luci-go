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

	"cloud.google.com/go/bigquery"
	"golang.org/x/sync/errgroup"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
)

type evalRun struct {
	Eval
	Result

	auth   *auth.Authenticator
	bq     *bigquery.Client
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

	var eligible int32
	var rejected int32
	for i := 0; i < r.Concurrency; i++ {
		w := &worker{evalRun: r}
		eg.Go(func() error {
			for rp := range rejectedPatchSetC {
				res, err := w.process(ctx, rp)
				if err != nil {
					return errors.Annotate(err, "failed to process patchset %s", &rp.Patchset).Err()
				}
				if res.eligible {
					atomic.AddInt32(&eligible, 1)
					if res.wouldReject {
						atomic.AddInt32(&rejected, 1)
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
			EligiblePatchSets: int(eligible),
			Rejected:          int(rejected),
		},
	}, nil
}

type worker struct {
	*evalRun
	out Output
}

type patchSetProcessingResult struct {
	eligible    bool
	wouldReject bool
}

func (w *worker) process(ctx context.Context, rp *RejectedPatchSet) (*patchSetProcessingResult, error) {
	files, err := w.gerrit.ChangedFiles(ctx, &rp.Patchset)
	switch {
	case clNotFound.In(err):
		// The CL is deleted  => not eligible.
		return &patchSetProcessingResult{eligible: false}, nil

	case err != nil:
		return nil, errors.Annotate(err, "failed to read changed files of %s", &rp.Patchset).Err()
	}

	// Compare the prediction to facts.
	in := &Input{ChangedFiles: files}
	res := &patchSetProcessingResult{wouldReject: false}
	for _, t := range rp.FailedTests {
		in.Test = t
		out, err := w.Algorithm(in)
		if err != nil {
			return nil, errors.Annotate(err, "RTS algorithm failed").Err()
		}
		res.eligible = true

		if out.ShouldRun {
			// At least one failed test would run => the bad patchset would be rejected.
			res.wouldReject = true
			break
		}
	}
	return res, nil
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
		logging.Infof(ctx, "Processing patchset: %d/%d\n", p.done, p.Total)
	}
	p.lastReport = clock.Now(ctx)
}
