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
	"sync/atomic"
	"time"

	"cloud.google.com/go/bigquery"
	"golang.org/x/sync/errgroup"

	"go.chromium.org/luci/auth"
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

func (r *evalRun) run(ctx context.Context) error {
	if err := r.Init(ctx); err != nil {
		return err
	}

	rejectedPatchSets, err := (&rejectedPatchSetQuery{evalRun: r}).Read(ctx)
	if err != nil {
		return err
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

	var eligable int32
	var rejected int32
	for i := 0; i < r.Concurrency; i++ {
		w := &worker{evalRun: r}
		eg.Go(func() error {
			for rp := range rejectedPatchSetC {
				res, err := w.process(ctx, rp)
				if err != nil {
					return errors.Annotate(err, "failed to process patchset %s", &rp.Patchset).Err()
				}
				if res.eligable {
					atomic.AddInt32(&eligable, 1)
					if res.wouldFail {
						atomic.AddInt32(&rejected, 1)
					}
				}
				progress.Done(ctx)
			}
			return ctx.Err()
		})
	}

	if err := eg.Wait(); err != nil {
		return err
	}
	logging.Infof(ctx, "Processed all patchsets")

	r.Result = Result{
		AnalyzedPatchSets: len(rejectedPatchSets),
		Safety: Safety{
			EligablePatchSets: int(eligable),
			Rejected:          int(rejected),
		},
	}
	return nil
}

type worker struct {
	*evalRun
	out Output
}

type patchSetProcessingResult struct {
	eligable    bool
	hasFailures bool
	wouldFail   bool
}

func (w *worker) process(ctx context.Context, rp *RejectedPatchSet) (*patchSetProcessingResult, error) {
	files, err := w.gerrit.ChangedFiles(ctx, &rp.Patchset)
	switch {
	case clNotFound.In(err):
		// The CL is deleted  => not eligable.
		return &patchSetProcessingResult{eligable: false}, nil

	case err != nil:
		return nil, errors.Annotate(err, "failed to read changed files of %s", &rp.Patchset).Err()
	}

	// Select tests.
	in := &Input{ChangedFiles: files}

	// Compare the prediction to facts.
	res := &patchSetProcessingResult{wouldFail: false}
	for _, t := range rp.FailedTests {
		in.Test = t
		out, err := w.Algorithm(in)
		if err != nil {
			return nil, errors.Annotate(err, "RTS algorithm failed").Err()
		}
		res.eligable = true

		if out.ShouldRun {
			// At least one failed test would run => the bad patchset would be rejected.
			res.wouldFail = true
		}
	}
	return res, nil
}
